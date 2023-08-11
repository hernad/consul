package dns

import (
	"fmt"
	"testing"

	"github.com/docker/go-connections/nat"
	"github.com/miekg/dns"
	"github.com/stretchr/testify/require"

	"github.com/hashicorp/consul/api"
	libassert "github.com/hashicorp/consul/test/integration/consul-container/libs/assert"
	libcluster "github.com/hashicorp/consul/test/integration/consul-container/libs/cluster"
	libservice "github.com/hashicorp/consul/test/integration/consul-container/libs/service"
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
	//clu2 := sp.Topology().Clusters["dc2"]

	//cluster1Client, err := sp.APIClientForNode("dc1", clu1.FirstClient().ID(), "")
	//require.NoError(t, err)
	//
	//cluster2Client, err := sp.APIClientForNode("dc2", clu2.FirstClient().ID(), "")
	//require.NoError(t, err)

	port := clu1.FirstClient().ExposedPort(8600)

	print(port)

	//dnsPort, err := nat.NewPort("udp", "8600")
	//require.NoError(t, err)
	//
	//client1Container := cluster1.Agents[0].GetPod()
	//client1MappedDNS, err := client1Container.MappedPort(context.Background(), dnsPort)
	//require.NoError(t, err)
	//
	//client2Container := cluster2.Agents[0].GetPod()
	//client2MappedDNS, err := client2Container.MappedPort(context.Background(), dnsPort)
	//
	//t1, t2 := mutualDNSCheck(t, client1MappedDNS, client2MappedDNS)
	//require.True(t, t1 && t2)

	// Need to make DNS request on 8600 to client
	// probably via the sidecar?

	// Don't need to test full suite of DNS just lookup the service in the other cluster from both sides

}

func createServices(t *testing.T, cluster *libcluster.Cluster, peerName string) *libservice.ConnectContainer {
	node := cluster.Agents[0]
	client := node.GetClient()
	// Create a service and proxy instance
	serviceOpts := &libservice.ServiceOpts{
		Name:     libservice.StaticServerServiceName,
		ID:       "static-server",
		HTTPPort: 8080,
		GRPCPort: 8079,
	}

	// Create a service and proxy instance
	_, _, err := libservice.CreateAndRegisterStaticServerAndSidecar(node, serviceOpts)
	require.NoError(t, err)

	libassert.CatalogServiceExists(t, client, "static-server-sidecar-proxy", nil)
	libassert.CatalogServiceExists(t, client, libservice.StaticServerServiceName, nil)

	// Create a client proxy instance with the server as an upstream
	clientConnectProxy, err := libservice.CreateAndRegisterStaticClientSidecar(node, "", false, false)
	require.NoError(t, err)

	libassert.CatalogServiceExists(t, client, "static-client-sidecar-proxy", nil)

	return clientConnectProxy
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
