// Copyright (c) 2018 Cisco and/or its affiliates.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at:
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package l2plugin_test

import (
	"testing"
	"git.fd.io/govpp.git/core"
	"github.com/ligato/vpp-agent/plugins/vpp/ifplugin/ifaceidx"
	"github.com/ligato/vpp-agent/plugins/vpp/l2plugin"
	"github.com/ligato/vpp-agent/idxvpp/nametoidx"
	"github.com/ligato/cn-infra/logging/logrus"
	"github.com/ligato/vpp-agent/tests/vppcallmock"
	"git.fd.io/govpp.git/adapter/mock"
	"github.com/ligato/cn-infra/logging"
	. "github.com/onsi/gomega"
	"github.com/ligato/vpp-agent/plugins/vpp/model/l2"
	l22 "github.com/ligato/vpp-agent/plugins/vpp/binapi/l2"
	"github.com/ligato/vpp-agent/plugins/vpp/binapi/vpe"
	"github.com/ligato/vpp-agent/plugins/vpp/l2plugin/l2idx"
)

func bdConfigTestInitialization(t *testing.T) (*vppcallmock.TestCtx, *core.Connection, ifaceidx.SwIfIndexRW, chan l2plugin.BridgeDomainStateMessage, *l2plugin.BDConfigurator, error) {
	RegisterTestingT(t)

	// Initialize notification channel
	notifChan := make(chan l2plugin.BridgeDomainStateMessage, 100)

	// Initialize sw if index
	nameToIdxSW := nametoidx.NewNameToIdx(logrus.DefaultLogger(), "ifaceidx_test", ifaceidx.IndexMetadata)
	swIfIndex := ifaceidx.NewSwIfIndex(nameToIdxSW)
	names := nameToIdxSW.ListNames()

	// Check if names were empty
	Expect(names).To(BeEmpty())

	// Create connection
	mockCtx := &vppcallmock.TestCtx{MockVpp: &mock.VppAdapter{}}
	connection, err := core.Connect(mockCtx.MockVpp)

	if err != nil {
		return nil, nil, nil, nil, nil, err
	}

	// Create plugin logger
	pluginLogger := logging.ForPlugin("testname", logrus.NewLogRegistry())

	// Test initialization
	bdConfiguratorPlugin := &l2plugin.BDConfigurator{}
	err = bdConfiguratorPlugin.Init(pluginLogger, connection, swIfIndex, notifChan, false)

	if err != nil {
		return nil, nil, nil, nil, nil, err
	}

	return mockCtx, connection, swIfIndex, notifChan, bdConfiguratorPlugin, nil
}

// Tests initialization and close of bridge domain configurator
func TestBDConfigurator_Init(t *testing.T) {
	_, conn, _, _, plugin, err := bdConfigTestInitialization(t)
	defer conn.Disconnect()
	Expect(err).To(BeNil())

	err = plugin.Close()
	Expect(err).To(BeNil())
}

// Tests configuration of bridge domain
func TestBDConfigurator_ConfigureBridgeDomain(t *testing.T) {
	ctx, conn, _, _, plugin, err := bdConfigTestInitialization(t)
	defer plugin.Close()
	defer conn.Disconnect()
	Expect(err).To(BeNil())

	ctx.MockVpp.MockReply(&l22.BridgeDomainAddDelReply{})
	ctx.MockVpp.MockReply(&l22.BdIPMacAddDelReply{})
	ctx.MockVpp.MockReply(&l22.BridgeDomainDetails{})
	ctx.MockVpp.MockReply(&vpe.ControlPingReply{})

	err = plugin.ConfigureBridgeDomain(&l2.BridgeDomains_BridgeDomain{
		Name:                "test",
		Flood:               true,
		UnknownUnicastFlood: true,
		Forward:             true,
		Learn:               true,
		ArpTermination:      true,
		MacAge:              20,
		Interfaces: []*l2.BridgeDomains_BridgeDomain_Interfaces{
			{
				Name:                    "if0",
				BridgedVirtualInterface: true,
				SplitHorizonGroup:       1,
			},
		},
		ArpTerminationTable: []*l2.BridgeDomains_BridgeDomain_ArpTerminationEntry{
			{
				IpAddress:   "192.168.0.1",
				PhysAddress: "01:23:45:67:89:ab",
			},
		},
	})

	Expect(err).To(BeNil())

	_, meta, found := plugin.GetBdIndexes().LookupIdx("test")
	Expect(found).To(BeTrue())
	Expect(meta.BridgeDomain.ArpTerminationTable).To(Not(BeEmpty()))

	table := meta.BridgeDomain.ArpTerminationTable[0]
	Expect(table.IpAddress).To(BeEquivalentTo("192.168.0.1"))
	Expect(table.PhysAddress).To(BeEquivalentTo("01:23:45:67:89:ab"))
}

// Tests modification of bridge domain (recreating it)
func TestBDConfigurator_ModifyBridgeDomainRecreate(t *testing.T) {
	ctx, conn, _, _, plugin, err := bdConfigTestInitialization(t)
	defer plugin.Close()
	defer conn.Disconnect()
	Expect(err).To(BeNil())

	ctx.MockVpp.MockReply(&l22.BridgeDomainAddDelReply{})
	ctx.MockVpp.MockReply(&l22.BdIPMacAddDelReply{})
	ctx.MockVpp.MockReply(&l22.BridgeDomainDetails{})
	ctx.MockVpp.MockReply(&vpe.ControlPingReply{})

	err = plugin.ModifyBridgeDomain(&l2.BridgeDomains_BridgeDomain{
		Name:                "test",
		Flood:               false,
		UnknownUnicastFlood: false,
		Forward:             false,
		Learn:               false,
		ArpTermination:      false,
		MacAge:              15,
		Interfaces: []*l2.BridgeDomains_BridgeDomain_Interfaces{
			{
				Name:                    "if0",
				BridgedVirtualInterface: true,
				SplitHorizonGroup:       1,
			},
		},
		ArpTerminationTable: []*l2.BridgeDomains_BridgeDomain_ArpTerminationEntry{
			{
				IpAddress:   "192.168.0.1",
				PhysAddress: "01:23:45:67:89:ab",
			},
		},
	}, &l2.BridgeDomains_BridgeDomain{
		Name:                "test",
		Flood:               true,
		UnknownUnicastFlood: true,
		Forward:             true,
		Learn:               true,
		ArpTermination:      true,
		MacAge:              20,
		Interfaces: []*l2.BridgeDomains_BridgeDomain_Interfaces{
			{
				Name:                    "if0",
				BridgedVirtualInterface: true,
				SplitHorizonGroup:       1,
			},
		},
		ArpTerminationTable: []*l2.BridgeDomains_BridgeDomain_ArpTerminationEntry{
			{
				IpAddress:   "192.168.0.1",
				PhysAddress: "01:23:45:67:89:ab",
			},
		},
	})

	Expect(err).To(BeNil())

	_, meta, found := plugin.GetBdIndexes().LookupIdx("test")
	Expect(found).To(BeTrue())
	Expect(meta.BridgeDomain.Flood).To(BeFalse())
	Expect(meta.BridgeDomain.UnknownUnicastFlood).To(BeFalse())
	Expect(meta.BridgeDomain.Forward).To(BeFalse())
	Expect(meta.BridgeDomain.Learn).To(BeFalse())
	Expect(meta.BridgeDomain.ArpTermination).To(BeFalse())
	Expect(meta.BridgeDomain.MacAge).To(BeEquivalentTo(15))
}

// Tests modification of bridge domain (bridge domain not found)
func TestBDConfigurator_ModifyBridgeDomainNotFound(t *testing.T) {
	ctx, conn, _, _, plugin, err := bdConfigTestInitialization(t)
	defer plugin.Close()
	defer conn.Disconnect()
	Expect(err).To(BeNil())

	ctx.MockVpp.MockReply(&l22.BridgeDomainAddDelReply{})
	ctx.MockVpp.MockReply(&l22.BdIPMacAddDelReply{})
	ctx.MockVpp.MockReply(&l22.BridgeDomainDetails{})
	ctx.MockVpp.MockReply(&vpe.ControlPingReply{})

	err = plugin.ModifyBridgeDomain(&l2.BridgeDomains_BridgeDomain{
		Name:                "test",
		Flood:               true,
		UnknownUnicastFlood: true,
		Forward:             true,
		Learn:               true,
		ArpTermination:      true,
		MacAge:              20,
		Interfaces: []*l2.BridgeDomains_BridgeDomain_Interfaces{
			{
				Name:                    "if1",
				BridgedVirtualInterface: true,
				SplitHorizonGroup:       1,
			},
		},
		ArpTerminationTable: []*l2.BridgeDomains_BridgeDomain_ArpTerminationEntry{
			{
				IpAddress:   "192.168.0.2",
				PhysAddress: "01:23:45:67:89:ac",
			},
		},
	}, &l2.BridgeDomains_BridgeDomain{
		Name:                "test",
		Flood:               true,
		UnknownUnicastFlood: true,
		Forward:             true,
		Learn:               true,
		ArpTermination:      true,
		MacAge:              20,
		Interfaces: []*l2.BridgeDomains_BridgeDomain_Interfaces{
			{
				Name:                    "if0",
				BridgedVirtualInterface: true,
				SplitHorizonGroup:       1,
			},
		},
		ArpTerminationTable: []*l2.BridgeDomains_BridgeDomain_ArpTerminationEntry{
			{
				IpAddress:   "192.168.0.1",
				PhysAddress: "01:23:45:67:89:ab",
			},
		},
	})

	Expect(err).To(BeNil())

	_, meta, found := plugin.GetBdIndexes().LookupIdx("test")
	Expect(found).To(BeTrue())
	Expect(meta.BridgeDomain.ArpTerminationTable).To(Not(BeEmpty()))

	table := meta.BridgeDomain.ArpTerminationTable[0]
	Expect(table.IpAddress).To(BeEquivalentTo("192.168.0.2"))
	Expect(table.PhysAddress).To(BeEquivalentTo("01:23:45:67:89:ac"))
}

// Tests modification of bridge domain (bridge domain already present)
func TestBDConfigurator_ModifyBridgeDomainFound(t *testing.T) {
	ctx, conn, _, _, plugin, err := bdConfigTestInitialization(t)
	defer plugin.Close()
	defer conn.Disconnect()
	Expect(err).To(BeNil())

	ctx.MockVpp.MockReply(&l22.BridgeDomainAddDelReply{})
	ctx.MockVpp.MockReply(&l22.BdIPMacAddDelReply{})
	ctx.MockVpp.MockReply(&l22.BridgeDomainDetails{})
	ctx.MockVpp.MockReply(&vpe.ControlPingReply{})

	plugin.GetBdIndexes().RegisterName("test", 0, l2idx.NewBDMetadata(&l2.BridgeDomains_BridgeDomain{
		Name:                "test",
		Flood:               true,
		UnknownUnicastFlood: true,
		Forward:             true,
		Learn:               true,
		ArpTermination:      true,
		MacAge:              20,
		Interfaces: []*l2.BridgeDomains_BridgeDomain_Interfaces{
			{
				Name:                    "if0",
				BridgedVirtualInterface: true,
				SplitHorizonGroup:       1,
			},
		},
		ArpTerminationTable: []*l2.BridgeDomains_BridgeDomain_ArpTerminationEntry{
			{
				IpAddress:   "192.168.0.1",
				PhysAddress: "01:23:45:67:89:ab",
			},
		},
	}, []string{"if0"}))

	err = plugin.ModifyBridgeDomain(&l2.BridgeDomains_BridgeDomain{
		Name:                "test",
		Flood:               true,
		UnknownUnicastFlood: true,
		Forward:             true,
		Learn:               true,
		ArpTermination:      true,
		MacAge:              20,
		Interfaces: []*l2.BridgeDomains_BridgeDomain_Interfaces{
			{
				Name:                    "if0",
				BridgedVirtualInterface: true,
				SplitHorizonGroup:       1,
			},
		},
		ArpTerminationTable: []*l2.BridgeDomains_BridgeDomain_ArpTerminationEntry{
			{
				IpAddress:   "192.168.0.2",
				PhysAddress: "01:23:45:67:89:ac",
			},
		},
	}, &l2.BridgeDomains_BridgeDomain{
		Name:                "test",
		Flood:               true,
		UnknownUnicastFlood: true,
		Forward:             true,
		Learn:               true,
		ArpTermination:      true,
		MacAge:              20,
		Interfaces: []*l2.BridgeDomains_BridgeDomain_Interfaces{
			{
				Name:                    "if0",
				BridgedVirtualInterface: false,
				SplitHorizonGroup:       1,
			},
		},
		ArpTerminationTable: []*l2.BridgeDomains_BridgeDomain_ArpTerminationEntry{
			{
				IpAddress:   "192.168.0.1",
				PhysAddress: "01:23:45:67:89:ab",
			},
		},
	})

	Expect(err).To(BeNil())

	_, meta, found := plugin.GetBdIndexes().LookupIdx("test")
	Expect(found).To(BeTrue())
	Expect(meta.BridgeDomain.ArpTerminationTable).To(Not(BeEmpty()))

	table := meta.BridgeDomain.ArpTerminationTable[0]
	Expect(table.IpAddress).To(BeEquivalentTo("192.168.0.2"))
	Expect(table.PhysAddress).To(BeEquivalentTo("01:23:45:67:89:ac"))
}

// Tests deletion of bridge domain
func TestBDConfigurator_DeleteBridgeDomain(t *testing.T) {
	ctx, conn, _, _, plugin, err := bdConfigTestInitialization(t)
	defer plugin.Close()
	defer conn.Disconnect()
	Expect(err).To(BeNil())

	ctx.MockVpp.MockReply(&l22.BridgeDomainAddDelReply{})

	plugin.GetBdIndexes().RegisterName("test", 0, l2idx.NewBDMetadata(&l2.BridgeDomains_BridgeDomain{
		Name:                "test",
		Flood:               true,
		UnknownUnicastFlood: true,
		Forward:             true,
		Learn:               true,
		ArpTermination:      true,
		MacAge:              20,
		Interfaces: []*l2.BridgeDomains_BridgeDomain_Interfaces{
			{
				Name:                    "if0",
				BridgedVirtualInterface: true,
				SplitHorizonGroup:       1,
			},
		},
		ArpTerminationTable: []*l2.BridgeDomains_BridgeDomain_ArpTerminationEntry{
			{
				IpAddress:   "192.168.0.1",
				PhysAddress: "01:23:45:67:89:ab",
			},
		},
	}, []string{"if0"}))

	err = plugin.DeleteBridgeDomain(&l2.BridgeDomains_BridgeDomain{
		Name:                "test",
		Flood:               true,
		UnknownUnicastFlood: true,
		Forward:             true,
		Learn:               true,
		ArpTermination:      true,
		MacAge:              20,
		Interfaces: []*l2.BridgeDomains_BridgeDomain_Interfaces{
			{
				Name:                    "if0",
				BridgedVirtualInterface: true,
				SplitHorizonGroup:       1,
			},
		},
		ArpTerminationTable: []*l2.BridgeDomains_BridgeDomain_ArpTerminationEntry{
			{
				IpAddress:   "192.168.0.1",
				PhysAddress: "01:23:45:67:89:ab",
			},
		},
	})

	Expect(err).To(BeNil())

	_, _, found := plugin.GetBdIndexes().LookupIdx("test")
	Expect(found).To(BeFalse())
}

// Tests resolving of created interface (not found)
func TestBDConfigurator_ResolveCreatedInterfaceNotFound(t *testing.T) {
	_, conn, _, _, plugin, err := bdConfigTestInitialization(t)
	defer plugin.Close()
	defer conn.Disconnect()
	Expect(err).To(BeNil())

	err = plugin.ResolveCreatedInterface("test", 0)
	Expect(err).To(BeNil())
}

// Tests resolving of created interface (present)
func TestBDConfigurator_ResolveCreatedInterfaceFound(t *testing.T) {
	ctx, conn, _, _, plugin, err := bdConfigTestInitialization(t)
	defer plugin.Close()
	defer conn.Disconnect()
	Expect(err).To(BeNil())

	ctx.MockVpp.MockReply(&l22.BridgeDomainDetails{
		BdID:         1,
		Flood:        1,
		UuFlood:      2,
		Forward:      3,
		Learn:        4,
		ArpTerm:      5,
		MacAge:       6,
		BdTag:        []byte("test"),
		BviSwIfIndex: 1,
		NSwIfs:       1,
		SwIfDetails: []l22.BridgeDomainSwIf{
			{
				SwIfIndex: 1,
				Context:   0,
				Shg:       20,
			},
		},
	})
	ctx.MockVpp.MockReply(&vpe.ControlPingReply{})

	plugin.GetBdIndexes().RegisterName("test", 0, l2idx.NewBDMetadata(&l2.BridgeDomains_BridgeDomain{
		Name:                "test",
		Flood:               true,
		UnknownUnicastFlood: true,
		Forward:             true,
		Learn:               true,
		ArpTermination:      true,
		MacAge:              20,
		Interfaces: []*l2.BridgeDomains_BridgeDomain_Interfaces{
			{
				Name:                    "test",
				BridgedVirtualInterface: true,
				SplitHorizonGroup:       1,
			},
		},
		ArpTerminationTable: []*l2.BridgeDomains_BridgeDomain_ArpTerminationEntry{
			{
				IpAddress:   "192.168.0.1",
				PhysAddress: "01:23:45:67:89:ab",
			},
		},
	}, []string{"test"}))

	err = plugin.ResolveCreatedInterface("test", 0)
	Expect(err).To(BeNil())

	_, meta, found := plugin.GetBdIndexes().LookupIdx("test")
	Expect(found).To(BeTrue())
	Expect(meta.ConfiguredInterfaces).To(Not(BeEmpty()))
	Expect(meta.ConfiguredInterfaces[0]).To(Equal("test"))
}

// Tests resolving of deleted interface (not found)
func TestBDConfigurator_ResolveDeletedInterfaceNotFound(t *testing.T) {
	_, conn, _, _, plugin, err := bdConfigTestInitialization(t)
	defer plugin.Close()
	defer conn.Disconnect()
	Expect(err).To(BeNil())

	err = plugin.ResolveDeletedInterface("test")
	Expect(err).To(BeNil())
}

// Tests resolving of deleted interface (present)
func TestBDConfigurator_ResolveDeletedInterfaceFound(t *testing.T) {
	ctx, conn, _, _, plugin, err := bdConfigTestInitialization(t)
	defer plugin.Close()
	defer conn.Disconnect()
	Expect(err).To(BeNil())

	ctx.MockVpp.MockReply(&l22.BridgeDomainDetails{
		BdID:         1,
		Flood:        1,
		UuFlood:      2,
		Forward:      3,
		Learn:        4,
		ArpTerm:      5,
		MacAge:       6,
		BdTag:        []byte("test"),
		BviSwIfIndex: 1,
		NSwIfs:       1,
		SwIfDetails: []l22.BridgeDomainSwIf{
			{
				SwIfIndex: 1,
				Context:   0,
				Shg:       20,
			},
		},
	})
	ctx.MockVpp.MockReply(&vpe.ControlPingReply{})

	plugin.GetBdIndexes().RegisterName("test", 0, l2idx.NewBDMetadata(&l2.BridgeDomains_BridgeDomain{
		Name:                "test",
		Flood:               true,
		UnknownUnicastFlood: true,
		Forward:             true,
		Learn:               true,
		ArpTermination:      true,
		MacAge:              20,
		Interfaces: []*l2.BridgeDomains_BridgeDomain_Interfaces{
			{
				Name:                    "test",
				BridgedVirtualInterface: true,
				SplitHorizonGroup:       1,
			},
		},
		ArpTerminationTable: []*l2.BridgeDomains_BridgeDomain_ArpTerminationEntry{
			{
				IpAddress:   "192.168.0.1",
				PhysAddress: "01:23:45:67:89:ab",
			},
		},
	}, []string{"test"}))

	err = plugin.ResolveDeletedInterface("test")
	Expect(err).To(BeNil())

	_, meta, found := plugin.GetBdIndexes().LookupIdx("test")
	Expect(found).To(BeTrue())
	Expect(meta.ConfiguredInterfaces).To(BeEmpty())
}
