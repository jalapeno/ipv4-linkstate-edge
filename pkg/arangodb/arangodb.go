package arangodb

import (
	"context"
	"encoding/json"

	driver "github.com/arangodb/go-driver"
	"github.com/cisco-open/jalapeno/topology/dbclient"
	"github.com/golang/glog"
	"github.com/jalapeno/ls-edge/kafkanotifier"
	"github.com/sbezverk/gobmp/pkg/bmp"
	"github.com/sbezverk/gobmp/pkg/message"
	"github.com/sbezverk/gobmp/pkg/tools"
)

type arangoDB struct {
	dbclient.DB
	*ArangoConn
	stop     chan struct{}
	vertex   driver.Collection
	edge     driver.Collection
	graph    driver.Collection
	notifier kafkanotifier.Event
}

// NewDBSrvClient returns an instance of a DB server client process
func NewDBSrvClient(arangoSrv, user, pass, dbname, vcn string, ecn string, notifier kafkanotifier.Event) (dbclient.Srv, error) {
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

	// Check if vertex collection exists, if not fail as Jalapeno topology is not running
	arango.vertex, err = arango.db.Collection(context.TODO(), vcn)
	if err != nil {
		return nil, err
	}
	// Check if edge collection exists, if not fail as Jalapeno topology is not running
	arango.edge, err = arango.db.Collection(context.TODO(), ecn)
	if err != nil {
		return nil, err
	}
	// Check if graph exists, if not fail as Jalapeno topology is not running
	arango.graph, err = arango.db.Collection(context.TODO(), arango.vertex.Name()+"_edge")
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
	query := "FOR d IN " + a.edge.Name() + " filter d.protocol_id != 7 RETURN d"
	cursor, err := a.db.Query(ctx, query, nil)
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
