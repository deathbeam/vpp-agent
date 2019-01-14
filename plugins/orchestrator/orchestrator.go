//  Copyright (c) 2019 Cisco and/or its affiliates.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at:
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.

package orchestrator

import (
	"sync"

	"github.com/gogo/protobuf/proto"
	"github.com/ligato/cn-infra/datasync"
	"github.com/ligato/cn-infra/datasync/kvdbsync/local"
	"github.com/ligato/cn-infra/infra"
	"github.com/ligato/cn-infra/rpc/grpc"
	"github.com/ligato/vpp-agent/api/models"
	"github.com/ligato/vpp-agent/plugins/govppmux"
	"golang.org/x/net/context"

	"github.com/ligato/vpp-agent/api"
	kvs "github.com/ligato/vpp-agent/plugins/kvscheduler/api"
)

// Registry is used for propagating transactions.
var Registry = local.DefaultRegistry

// Plugin implements sync service for GRPC.
type Plugin struct {
	Deps

	genericConfigurator *genericConfigurator

	// datasync channels
	changeChan   chan datasync.ChangeEvent
	resyncChan   chan datasync.ResyncEvent
	watchDataReg datasync.WatchRegistration

	mu      sync.Mutex
	localDB map[string]models.ProtoItem
}

// Deps represents dependencies for the plugin.
type Deps struct {
	infra.PluginDeps

	GoVPP       govppmux.API
	GRPC        grpc.Server
	KVScheduler kvs.KVScheduler
	Watcher     datasync.KeyValProtoWatcher
}

// Init registers the service to GRPC server.
func (p *Plugin) Init() error {
	// initialize datasync channels
	p.resyncChan = make(chan datasync.ResyncEvent)
	p.changeChan = make(chan datasync.ChangeEvent)

	// register grpc service
	p.genericConfigurator = &genericConfigurator{
		log:  p.Log,
		orch: p,
	}
	api.RegisterGenericConfiguratorServer(p.GRPC.GetServer(), p.genericConfigurator)
	//reflection.Register(p.GRPC.GetServer())

	p.localDB = map[string]models.ProtoItem{}

	return nil
}

// AfterInit subscribes to known NB prefixes.
func (p *Plugin) AfterInit() error {
	go p.watchEvents()

	var err error
	p.watchDataReg, err = p.Watcher.Watch("orchestrator",
		p.changeChan, p.resyncChan, p.KVScheduler.GetRegisteredNBKeyPrefixes()...)
	if err != nil {
		return err
	}

	return nil
}

func (p *Plugin) watchEvents() {
	for {
		select {
		case e := <-p.changeChan:
			p.Log.Debugf("=> received CHANGE event (%v changes)", len(e.GetChanges()))

			/*var kvPairs []kvs.KeyValuePair
			for _, x := range e.GetChanges() {
				p.Log.Debugf(" - %v: %q (rev: %v)",
					x.GetChangeType(), x.GetKey(), x.GetRevision())

				var val proto.Message
				if x.GetChangeType() != datasync.Delete {
					val = x.G
				}

				kvPairs = append(kvPairs, ProtoWatchResp{
					Key:   x.GetKey(),
					Val: val,
				})
			}*/
			kvPairs := e.GetChanges()

			ctx := context.Background()
			//ctx = kvs.WithRetry(ctx, time.Second, true)
			err, _ := p.PushData(ctx, kvPairs)
			e.Done(err)

			/*txn := p.KVScheduler.StartNBTransaction()
			for _, x := range e.GetChanges() {
				p.Log.Debugf(" - %v: %q (rev: %v)",
					x.GetChangeType(), x.GetKey(), x.GetRevision())
				if x.GetChangeType() == datasync.Delete {
					txn.SetValue(x.GetKey(), nil)
				} else {
					txn.SetValue(x.GetKey(), x)
				}
			}

			ctx := context.Background()
			//ctx = kvs.WithRetry(ctx, time.Second, true)

			kvErrs, err := txn.Commit(ctx)
			if err != nil {
				p.Log.Errorf("transaction failed: %v", err)
			} else if len(kvErrs) > 0 {
				p.Log.Warnf("transaction finished with %d errors: %+v", len(kvErrs), kvErrs)
			} else {
				p.Log.Infof("transaction successful")
			}
			e.Done(err)*/

		case e := <-p.resyncChan:
			p.Log.Debugf("=> received RESYNC event (%v prefixes)", len(e.GetValues()))

			var kvPairs []datasync.ProtoWatchResp

			n := 0
			for prefix, iter := range e.GetValues() {
				var keyVals []datasync.KeyVal
				for x, done := iter.GetNext(); !done; x, done = iter.GetNext() {
					kvPairs = append(kvPairs, &ProtoWatchResp{
						Key:  x.GetKey(),
						lazy: x,
					})
					keyVals = append(keyVals, x)
					n++
				}
				if len(keyVals) > 0 {
					p.Log.Debugf(" - Resync: %q (%v items)", prefix, len(keyVals))
				} else {
					p.Log.Debugf(" - Resync: %q", prefix)
				}
				for _, x := range keyVals {
					p.Log.Debugf("\t - %q: (rev: %v)", x.GetKey(), x.GetRevision())
				}
			}
			p.Log.Debugf("Resync with %d items", n)

			ctx := context.Background()
			ctx = kvs.WithResync(ctx, kvs.FullResync, true)
			//ctx = kvs.WithRetry(ctx, time.Second, true)
			err, _ := p.PushData(ctx, kvPairs)
			e.Done(err)

			/*n := 0
			txn := p.KVScheduler.StartNBTransaction()
			for prefix, iter := range e.GetValues() {
				var keyVals []datasync.KeyVal
				for x, done := iter.GetNext(); !done; x, done = iter.GetNext() {
					keyVals = append(keyVals, x)
					txn.SetValue(x.GetKey(), x)
					n++
				}
				if len(keyVals) > 0 {
					p.Log.Debugf(" - Resync: %q (%v items)", prefix, len(keyVals))
				} else {
					p.Log.Debugf(" - Resync: %q", prefix)
				}
				for _, x := range keyVals {
					p.Log.Debugf("\t - %q: (rev: %v)", x.GetKey(), x.GetRevision())
				}
			}
			p.Log.Debugf("Resyncing %d items", n)

			ctx := context.Background()
			//ctx = kvs.WithRetry(ctx, time.Second, true)
			ctx = kvs.WithResync(ctx, kvs.FullResync, true)

			kvErrs, err := txn.Commit(ctx)
			if err != nil {
				p.Log.Errorf("transaction failed: %v", err)
			} else if len(kvErrs) > 0 {
				p.Log.Warnf("transaction finished with %d errors: %+v", len(kvErrs), kvErrs)
			} else {
				p.Log.Infof("transaction successful")
			}
			e.Done(err)*/
		}
	}
}

type ProtoWatchResp struct {
	Key  string
	Val  proto.Message
	lazy datasync.LazyValue
}

func (item *ProtoWatchResp) GetRevision() int64 {
	return 0
}

func (item *ProtoWatchResp) GetPrevValue(prevValue proto.Message) (prevValueExist bool, err error) {
	return false, nil
}

func (item *ProtoWatchResp) GetChangeType() datasync.Op {
	if item.Val == nil && item.lazy == nil {
		return datasync.Delete
	}
	return datasync.Put
}

func (item *ProtoWatchResp) GetKey() string {
	return item.Key
}

func (item *ProtoWatchResp) GetValue(out proto.Message) error {
	if item.Val != nil {
		proto.Merge(out, item.Val)
	} else if item.lazy != nil {
		return item.lazy.GetValue(out)
	}
	return nil
}

func (p *Plugin) ListData() map[string]models.ProtoItem {
	return p.localDB
}

// PushData ...
func (p *Plugin) PushData(ctx context.Context, kvPairs []datasync.ProtoWatchResp) (err error, kvErrs []kvs.KeyWithError) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if typ, _ := kvs.IsResync(ctx); typ == kvs.FullResync {
		p.localDB = map[string]models.ProtoItem{}
	}

	txn := p.KVScheduler.StartNBTransaction()

	for _, kv := range kvPairs {
		p.Log.Debugf(" - %v: %q (rev: %v)",
			kv.GetChangeType(), kv.GetKey(), kv.GetRevision())

		if kv.GetChangeType() == datasync.Delete {
			txn.SetValue(kv.GetKey(), nil)
			delete(p.localDB, kv.GetKey())
		} else {
			txn.SetValue(kv.GetKey(), kv)
			p.localDB[kv.GetKey()] = kv.(*ProtoWatchResp).Val
		}
	}

	seqID, err := txn.Commit(ctx)
	if err != nil {
		p.Log.Errorf("transaction failed (seq=%d): %v", seqID, err)
		return err, nil
	} else {
		if txErr, ok := err.(*kvs.TransactionError); ok && len(txErr.GetKVErrors()) > 0 {
			kvErrs = txErr.GetKVErrors()
			p.Log.Warnf("transaction finished with %d errors: %+v", len(kvErrs), kvErrs)
		}
		p.Log.Infof("transaction successful (seq=%d)", seqID)
		return err, kvErrs
	}

	return nil, nil
}
