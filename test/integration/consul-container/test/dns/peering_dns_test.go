package dns

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/docker/go-connections/nat"
	"github.com/miekg/dns"
	"github.com/stretchr/testify/require"

	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/consul/testing/deployer/sprawl/sprawltest"
	"github.com/hashicorp/consul/testing/deployer/topology"
)

// TestDNS_PeeringDNSTest
// This test creates two peered datacenters then ensures that each datacenter is able to
// make cross datacenter DNS requests when
// 1. The default token is configured with service:read and node:read permissions
// 2. The default token and the DNS token have service:read and node:read permissions
// 3. The default token has no permissions and the DNS token has service:read and node:read permissions
// 4. The default token is not set and the DNS token has service:read and node:read permissions
//
// The test then also validates that DNS does not work in the following cases
// 1. The default token and the DNS token are unset
// 2. The default token has no permissions and the DNS token is unset
// 3. The default token is unset and the DNS token has no permissions
// 4. The default token and the DNS token have no permissions
//
// Test Steps
// Setup
// * Set up peering topology; 2 clusters, 1 service from each cluster exported to the other
//
// Part 1 (Positive cases)
// * configure default token with service:read and node:read permissions and validate DNS each way
// * configure the DNS token with service:read and node:read permissions and validate DNS
// * configure default token with no permissions and validate DNS
// * unset default token and validate DNS
//
// Part 2 (Negative cases)
// * unset DNS token and validate DNS does not work
// * set default token with no permissions and validate DNS does not work
// * set DNS token with no permissions and validate that DNS does not work
// * unset default token and validate that DNS does not work
func TestDNS_PeeringDNSTest(t *testing.T) {
	t.Parallel()

	cfg := &topology.Config{
		Networks: []*topology.Network{
			{Name: "dc1"},
			{Name: "dc2"},
			{Name: "wan", Type: "wan"},
		},
		Clusters: []*topology.Cluster{
			{
				Name: "dc1",
				Nodes: []*topology.Node{
					{
						Kind: topology.NodeKindServer,
						Name: "dc1-server1",
						Addresses: []*topology.Address{
							{Network: "dc1"},
							{Network: "wan"},
						},
					},
					{
						Kind: topology.NodeKindClient,
						Name: "dc1-client1",
						Services: []*topology.Service{
							{
								ID:             topology.ServiceID{Name: "ping"},
								Image:          "rboyer/pingpong:latest",
								Port:           8080,
								EnvoyAdminPort: 19000,
								Command: []string{
									"-bind", "0.0.0.0:8080",
									"-dial", "127.0.0.1:9090",
									"-pong-chaos",
									"-dialfreq", "250ms",
									"-name", "ping",
								},
								Upstreams: []*topology.Upstream{{
									ID:        topology.ServiceID{Name: "pong"},
									LocalPort: 9090,
									Peer:      "peer-dc2-default",
								}},
							},
						},
					},
				},
				InitialConfigEntries: []api.ConfigEntry{
					&api.ExportedServicesConfigEntry{
						Name: "default",
						Services: []api.ExportedService{{
							Name: "ping",
							Consumers: []api.ServiceConsumer{{
								Peer: "peer-dc2-default",
							}},
						}},
					},
				},
			},
			{
				Name: "dc2",
				Nodes: []*topology.Node{
					{
						Kind: topology.NodeKindServer,
						Name: "dc2-server1",
						Addresses: []*topology.Address{
							{Network: "dc2"},
							{Network: "wan"},
						},
					},
					{
						Kind: topology.NodeKindDataplane,
						Name: "dc2-client1",
						Services: []*topology.Service{
							{
								ID:             topology.ServiceID{Name: "pong"},
								Image:          "rboyer/pingpong:latest",
								Port:           8080,
								EnvoyAdminPort: 19000,
								Command: []string{
									"-bind", "0.0.0.0:8080",
									"-dial", "127.0.0.1:9090",
									"-pong-chaos",
									"-dialfreq", "250ms",
									"-name", "pong",
								},
								Upstreams: []*topology.Upstream{{
									ID:        topology.ServiceID{Name: "ping"},
									LocalPort: 9090,
									Peer:      "peer-dc1-default",
								}},
							},
						},
					},
				},
				InitialConfigEntries: []api.ConfigEntry{
					&api.ExportedServicesConfigEntry{
						Name: "default",
						Services: []api.ExportedService{{
							Name: "ping",
							Consumers: []api.ServiceConsumer{{
								Peer: "peer-dc2-default",
							}},
						}},
					},
				},
			},
		},
		Peerings: []*topology.Peering{{
			Dialing: topology.PeerCluster{
				Name: "dc1",
			},
			Accepting: topology.PeerCluster{
				Name: "dc2",
			},
		}},
	}

	// launch clusters
	sp := sprawltest.Launch(t, cfg)
	clu1 := sp.Topology().Clusters["dc1"]
	clu2 := sp.Topology().Clusters["dc2"]

	client1DNSPortNum := clu1.FirstClient().ExposedPort(8600)
	client2DNSPortNum := clu2.FirstClient().ExposedPort(8600)

	client1DNSPort, err := nat.NewPort("udp", strconv.Itoa(client1DNSPortNum))
	require.NoError(t, err)

	client2DNSPort, err := nat.NewPort("udp", strconv.Itoa(client2DNSPortNum))
	require.NoError(t, err)

	t1, t2 := mutualDNSCheck(t, client1DNSPort, client2DNSPort)
	require.True(t, t1 && t2)

	// TODO: Adjust token configurations for test cases
}

func mutualDNSCheck(t *testing.T, cluster1Port, cluster2Port nat.Port) (bool, bool) {
	m := new(dns.Msg)
	m.SetQuestion("static-server.service.dialing-to-acceptor.peer.consul.", dns.TypeSRV)

	c := new(dns.Client)

	addr1 := fmt.Sprintf("127.0.0.1:%d", cluster1Port.Int())
	reply1, _, err := c.Exchange(m, addr1)
	require.NoError(t, err)

	m = new(dns.Msg)
	m.SetQuestion("static-server.service.accepting-to-dialer.peer.consul.", dns.TypeSRV)

	addr2 := fmt.Sprintf("127.0.0.1:%d", cluster2Port.Int())
	reply2, _, err := c.Exchange(m, addr2)
	require.NoError(t, err)

	return len(reply1.Answer) > 0, len(reply2.Answer) > 0
}
