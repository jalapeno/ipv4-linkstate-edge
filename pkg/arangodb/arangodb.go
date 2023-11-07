package arangodb

import (
	"context"
	"encoding/json"

	driver "github.com/arangodb/go-driver"
	"github.com/cisco-open/jalapeno/topology/dbclient"
	"github.com/golang/glog"
	"github.com/jalapeno/linkstate-edge-v4/pkg/kafkanotifier"
	"github.com/sbezverk/gobmp/pkg/bmp"
	"github.com/sbezverk/gobmp/pkg/message"
	"github.com/sbezverk/gobmp/pkg/tools"
)

type arangoDB struct {
	dbclient.DB
	*ArangoConn
	stop      chan struct{}
	lsnode    driver.Collection
	lslink    driver.Collection
	lsprefix  driver.Collection
	graph     driver.Collection
	lsnodeExt driver.Collection
	lstopo    driver.Graph
	notifier  kafkanotifier.Event
}

// NewDBSrvClient returns an instance of a DB server client process
func NewDBSrvClient(arangoSrv, user, pass, dbname, lsnode string, lslink string, lsprefix string, lsnodeExt string, lstopo string, notifier kafkanotifier.Event) (dbclient.Srv, error) {
	if err := tools.URLAddrValidation(arangoSrv); err != nil {
		return nil, err
	}
	arangoConn, err := NewArango(ArangoConfig{
		URL:      arangoSrv,
		User:     user,
		Password: pass,
		Database: dbname,
	})
	if err != nil {
		return nil, err
	}
	arango := &arangoDB{
		stop: make(chan struct{}),
	}
	arango.DB = arango
	arango.ArangoConn = arangoConn
	if notifier != nil {
		arango.notifier = notifier
	}

	// Check if original ls_node collection exists, if not fail as Jalapeno topology is not running
	arango.lsnode, err = arango.db.Collection(context.TODO(), lsnode)
	if err != nil {
		return nil, err
	}
	// Check if ls_link edge collection exists, if not fail as Jalapeno topology is not running
	arango.lslink, err = arango.db.Collection(context.TODO(), lslink)
	if err != nil {
		return nil, err
	}

	// Check if ls_prefix collection exists, if not fail as Jalapeno topology is not running
	arango.lsprefix, err = arango.db.Collection(context.TODO(), lsprefix)
	if err != nil {
		return nil, err
	}

	//Check if ls_node_ext collection exists, if not fail as Jalapeno topology is not running
	arango.lsnodeExt, err = arango.db.Collection(context.TODO(), lsnodeExt)
	if err != nil {
		return nil, err
	}

	// check for lsnode_extended collection
	found, err := arango.db.CollectionExists(context.TODO(), lsnodeExt)
	if err != nil {
		return nil, err
	}
	if found {
		c, err := arango.db.Collection(context.TODO(), lsnodeExt)
		if err != nil {
			return nil, err
		}
		glog.Infof("ls_node_extended collection found %s, proceed to processing data", c)
	}

	// check for ls topology graph
	found, err = arango.db.GraphExists(context.TODO(), lstopo)
	if err != nil {
		return nil, err
	}
	if found {
		c, err := arango.db.Graph(context.TODO(), lstopo)
		if err != nil {
			return nil, err
		}
		glog.Infof("found graph %s", c)

	} else {
		// create graph
		var edgeDefinition driver.EdgeDefinition
		edgeDefinition.Collection = "ls_topology_v4"
		edgeDefinition.From = []string{"ls_node_extended"}
		edgeDefinition.To = []string{"ls_node_extended"}
		var options driver.CreateGraphOptions
		options.OrphanVertexCollections = []string{"ls_srv6_sid", "ls_prefix"}
		options.EdgeDefinitions = []driver.EdgeDefinition{edgeDefinition}

		arango.lstopo, err = arango.db.CreateGraph(context.TODO(), lstopo, &options)
		if err != nil {
			return nil, err
		}
	}

	// check if graph exists, if not fail as processor has failed to create graph
	arango.graph, err = arango.db.Collection(context.TODO(), "ls_topology_v4")
	if err != nil {
		return nil, err
	}
	return arango, nil
}

func (a *arangoDB) Start() error {
	if err := a.loadEdge(); err != nil {
		return err
	}
	glog.Infof("Connected to arango database, starting monitor")

	return nil
}

func (a *arangoDB) Stop() error {
	close(a.stop)

	return nil
}

func (a *arangoDB) GetInterface() dbclient.DB {
	return a.DB
}

func (a *arangoDB) GetArangoDBInterface() *ArangoConn {
	return a.ArangoConn
}

func (a *arangoDB) StoreMessage(msgType dbclient.CollectionType, msg []byte) error {
	event := &kafkanotifier.EventMessage{}
	if err := json.Unmarshal(msg, event); err != nil {
		return err
	}
	glog.V(9).Infof("Received event from topology: %+v", *event)
	event.TopicType = msgType
	switch msgType {
	case bmp.LSLinkMsg:
		return a.lsLinkHandler(event)
	}
	switch msgType {
	case bmp.LSPrefixMsg:
		return a.lsprefixHandler(event)
	}
	return nil
}

func (a *arangoDB) loadEdge() error {
	ctx := context.TODO()

	lslinkquery := "for l in " + a.lslink.Name() + " filter l.protocol_id != 7 RETURN l"
	cursor, err := a.db.Query(ctx, lslinkquery, nil)
	if err != nil {
		return err
	}
	defer cursor.Close()
	for {
		var p message.LSLink
		meta, err := cursor.ReadDocument(ctx, &p)
		//glog.Infof("processing lslink document: %+v", p)
		if driver.IsNoMoreDocuments(err) {
			break
		} else if err != nil {
			return err
		}
		if err := a.processLSLinkEdge(ctx, meta.Key, &p); err != nil {
			glog.Errorf("failed to process key: %s with error: %+v", meta.Key, err)
			continue
		}
	}

	lspfxquery := "for l in " + a.lsprefix.Name() + //" filter l.mt_id_tlv == null return l"
		" filter l.mt_id_tlv.mt_id != 2 && l.prefix_len != 30 && " +
		"l.prefix_len != 31 && l.prefix_len != 32 return l"
	cursor, err = a.db.Query(ctx, lspfxquery, nil)
	if err != nil {
		return err
	}
	defer cursor.Close()
	for {
		var p message.LSPrefix
		meta, err := cursor.ReadDocument(ctx, &p)
		//glog.Infof("processing lsprefix document: %+v", p)
		if driver.IsNoMoreDocuments(err) {
			break
		} else if err != nil {
			return err
		}
		if err := a.processLSPrefixEdge(ctx, meta.Key, &p); err != nil {
			glog.Errorf("failed to process key: %s with error: %+v", meta.Key, err)
			continue
		}
	}

	// loopbacksquery := "for l in " + a.lsprefix.Name() + " filter l.prefix_len == 32"
	// cursor, err = a.db.Query(ctx, loopbacksquery, nil)
	// if err != nil {
	// 	return err
	// }
	// defer cursor.Close()
	// for {
	// 	var p message.LSPrefix
	// 	meta, err := cursor.ReadDocument(ctx, &p)
	// 	//glog.Infof("processing lslink document: %+v", p)
	// 	if driver.IsNoMoreDocuments(err) {
	// 		break
	// 	} else if err != nil {
	// 		return err
	// 	}
	// 	if err := a.processLoopbacks(ctx, meta.Key, &p); err != nil {
	// 		glog.Errorf("failed to process key: %s with error: %+v", meta.Key, err)
	// 		continue
	// 	}
	// }

	return nil
}
