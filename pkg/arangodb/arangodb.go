package arangodb

import (
	"context"
	"encoding/json"
	"fmt"

	driver "github.com/arangodb/go-driver"
	"github.com/cisco-open/jalapeno/topology/dbclient"
	"github.com/golang/glog"
	"github.com/jalapeno/ls-edge/pkg/kafkanotifier"
	"github.com/sbezverk/gobmp/pkg/bmp"
	"github.com/sbezverk/gobmp/pkg/message"
	"github.com/sbezverk/gobmp/pkg/tools"
)

type arangoDB struct {
	dbclient.DB
	*ArangoConn
	stop      chan struct{}
	lsnode    driver.Collection
	edge      driver.Collection
	graph     driver.Collection
	lsnodeExt driver.Collection
	lstopo    driver.Graph
	notifier  kafkanotifier.Event
}

// NewDBSrvClient returns an instance of a DB server client process
func NewDBSrvClient(arangoSrv, user, pass, dbname, lsnode string, ecn string, lsnodeExt string, lstopo string, notifier kafkanotifier.Event) (dbclient.Srv, error) {
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
	// Check if edge collection exists, if not fail as Jalapeno topology is not running
	arango.edge, err = arango.db.Collection(context.TODO(), ecn)
	if err != nil {
		return nil, err
	}

	// check for lsnode collection, if it doesn't exist, create it
	found, err := arango.db.CollectionExists(context.TODO(), lsnodeExt)
	if err != nil {
		return nil, err
	}
	if found {
		c, err := arango.db.Collection(context.TODO(), lsnodeExt)
		if err != nil {
			return nil, err
		}
		glog.Infof("ls_node_extended collection found, proceed to processing data")

		if err := c.Remove(context.TODO()); err != nil {
			return nil, err
		}
	}
	// create ls_node_extended collection
	var lsnode_options = &driver.CreateCollectionOptions{ /* ... */ }
	//glog.Infof("ls_node_extended collection not found, creating collection")
	arango.lsnodeExt, err = arango.db.CreateCollection(context.TODO(), "ls_node_extended", lsnode_options)
	if err != nil {
		return nil, err
	}

	// check if collection exists, if not fail as processor has failed to create collection
	arango.lsnodeExt, err = arango.db.Collection(context.TODO(), lsnodeExt)
	if err != nil {
		return nil, fmt.Errorf("failed to create lsnode collection")
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
		if err := c.Remove(context.TODO()); err != nil {
			return nil, err
		}
	}
	// create graph
	var edgeDefinition driver.EdgeDefinition
	edgeDefinition.Collection = "ls_topology"
	edgeDefinition.From = []string{"ls_node_extended"}
	edgeDefinition.To = []string{"ls_node_extended"}
	var options driver.CreateGraphOptions
	options.OrphanVertexCollections = []string{"ls_srv6_sid", "ls_prefix"}
	options.EdgeDefinitions = []driver.EdgeDefinition{edgeDefinition}

	arango.lstopo, err = arango.db.CreateGraph(context.TODO(), lstopo, &options)
	if err != nil {
		return nil, err
	}
	// check if graph exists, if not fail as processor has failed to create graph
	arango.graph, err = arango.db.Collection(context.TODO(), "ls_topology")
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

	return nil
}

func (a *arangoDB) loadEdge() error {
	ctx := context.TODO()

	// copy ls_node data into new lsnode collection
	glog.Infof("copy ls_node into ls_node_extended")
	lsn_query := "for l in " + a.lsnode.Name() + " insert l in " + a.lsnodeExt.Name() + ""
	cursor, err := a.db.Query(ctx, lsn_query, nil)
	if err != nil {
		return err
	}
	defer cursor.Close()

	// BGP-LS generates a level-1 and a level-2 entry for level-1-2 nodes
	// remove duplicate entries in the lsnodeExt collection
	dup_query := "LET duplicates = ( FOR d IN " + a.lsnodeExt.Name() +
		" COLLECT id = d.igp_router_id, area = d.area_id WITH COUNT INTO count " +
		" FILTER count > 1 RETURN { id: id, area: area, count: count }) " +
		"FOR d IN duplicates FOR m IN ls_node_extended FILTER d.id == m.igp_router_id " +
		"RETURN m "
	pcursor, err := a.db.Query(ctx, dup_query, nil)
	if err != nil {
		return err
	}
	defer pcursor.Close()
	for {
		var doc duplicateNode
		dupe, err := pcursor.ReadDocument(ctx, &doc)

		if err != nil {
			if !driver.IsNoMoreDocuments(err) {
				return err
			}
			break
		}
		fmt.Printf("Got doc with key '%s' from query\n", dupe.Key)

		if doc.ProtocolID == 1 {
			glog.Infof("remove level-1 duplicate node: %s + igp id: %s area id: %s protocol id: %v +  ", doc.Key, doc.IGPRouterID, doc.AreaID, doc.ProtocolID)
			if _, err := a.lsnodeExt.RemoveDocument(ctx, doc.Key); err != nil {
				if !driver.IsConflict(err) {
					return err
				}
			}
		}
		if doc.ProtocolID == 2 {
			update_query := "for l in " + a.lsnodeExt.Name() + " filter l._key == " + "\"" + doc.Key + "\"" +
				" UPDATE l with { protocol: " + "\"" + "ISIS Level 1-2" + "\"" + " } in " + a.lsnodeExt.Name() + ""
			cursor, err := a.db.Query(ctx, update_query, nil)
			glog.Infof("update query: %s ", update_query)
			if err != nil {
				return err
			}
			defer cursor.Close()
		}
	}
	query := "FOR d IN " + a.edge.Name() + " filter d.protocol_id != 7 RETURN d"
	cursor, err = a.db.Query(ctx, query, nil)
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
		if err := a.processEdge(ctx, meta.Key, &p); err != nil {
			glog.Errorf("failed to process key: %s with error: %+v", meta.Key, err)
			continue
		}
	}

	return nil
}
