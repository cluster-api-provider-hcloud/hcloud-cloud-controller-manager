/*
Copyright 2018 Hetzner Cloud GmbH.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package hcloud

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/cluster-api-provider-hcloud/hcloud-cloud-controller-manager/internal/hcops"
	"github.com/hetznercloud/hcloud-go/hcloud"
	hrobot "github.com/nl2go/hrobot-go"
	cloudprovider "k8s.io/cloud-provider"
	klog "k8s.io/klog/v2"
)

const (
	hrobotUserENVVar                 = "HROBOT_USER"
	hrobotPassENVVar                 = "HROBOT_PASS"
	hrobotPeriodENVVar               = "HROBOT_PERIOD"
	hcloudTokenENVVar                = "HCLOUD_TOKEN"
	hcloudEndpointENVVar             = "HCLOUD_ENDPOINT"
	hcloudNetworkENVVar              = "HCLOUD_NETWORK"
	hcloudDebugENVVar                = "HCLOUD_DEBUG"
	hcloudLoadBalancersEnabledENVVar = "HCLOUD_LOAD_BALANCERS_ENABLED"
	nodeNameENVVar                   = "NODE_NAME"
	providerName                     = "hcloud"
	providerVersion                  = "v1.8.0"
)

var (
	hrobotPeriod = 180
)

type commonClient struct {
	Hrobot hrobot.RobotClient
	Hcloud *hcloud.Client
}
type cloud struct {
	client       commonClient
	instances    *instances
	zones        *zones
	loadBalancer *loadBalancers
	networkID    int
}

type HrobotServer struct {
	ID     int
	Name   string
	Type   string
	Zone   string
	Region string
	IP     net.IP
}

var hrobotServers []HrobotServer

func newCloud(config io.Reader) (cloudprovider.Interface, error) {
	const op = "hcloud/newCloud"

	token := os.Getenv(hcloudTokenENVVar)
	if token == "" {
		return nil, fmt.Errorf("environment variable %q is required", hcloudTokenENVVar)
	}
	if len(token) != 64 {
		return nil, fmt.Errorf("entered token is invalid (must be exactly 64 characters long)")
	}
	nodeName := os.Getenv(nodeNameENVVar)
	if nodeName == "" {
		return nil, fmt.Errorf("environment variable %q is required", nodeNameENVVar)
	}

	opts := []hcloud.ClientOption{
		hcloud.WithToken(token),
		hcloud.WithApplication("hcloud-cloud-controller", providerVersion),
	}
	if os.Getenv(hcloudDebugENVVar) == "true" {
		opts = append(opts, hcloud.WithDebugWriter(os.Stderr))
	}
	if endpoint := os.Getenv(hcloudEndpointENVVar); endpoint != "" {
		opts = append(opts, hcloud.WithEndpoint(endpoint))
	}

	// hetzner robot get auth from env
	user := os.Getenv(hrobotUserENVVar)
	if user == "" {
		return nil, fmt.Errorf("environment variable %q is required", hrobotUserENVVar)
	}
	pass := os.Getenv(hrobotPassENVVar)
	if pass == "" {
		return nil, fmt.Errorf("environment variable %q is required", hrobotPassENVVar)
	}

	period := os.Getenv(hrobotPeriodENVVar)
	if period == "" {
		hrobotPeriod = 180
	} else {
		hrobotPeriod, _ = strconv.Atoi(period)
	}

	var client commonClient
	client.Hcloud = hcloud.NewClient(opts...)
	client.Hrobot = hrobot.NewBasicAuthClient(user, pass)

	readHrobotServers(client.Hrobot)

	var networkID int
	if v, ok := os.LookupEnv(hcloudNetworkENVVar); ok {
		n, _, err := client.Hcloud.Network.Get(context.Background(), v)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", op, err)
		}
		if n == nil {
			return nil, fmt.Errorf("%s: Network %s not found", op, v)
		}
		networkID = n.ID
	}
	if networkID == 0 {
		klog.Infof("%s: %s empty", op, hcloudNetworkENVVar)
	}

	_, _, err := client.Hcloud.Server.List(context.Background(), hcloud.ServerListOpts{})
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	fmt.Printf("Hetzner Cloud k8s cloud controller %s started\n", providerVersion)

	lbOps := &hcops.LoadBalancerOps{
		LBClient:      &client.Hcloud.LoadBalancer,
		ActionClient:  &client.Hcloud.Action,
		NetworkClient: &client.Hcloud.Network,
		NetworkID:     networkID,
	}

	loadBalancers := newLoadBalancers(lbOps, &client.Hcloud.LoadBalancer, &client.Hcloud.Action)
	if os.Getenv(hcloudLoadBalancersEnabledENVVar) == "false" {
		loadBalancers = nil
	}
	return &cloud{
		client:       client,
		zones:        newZones(client, nodeName),
		instances:    newInstances(client),
		loadBalancer: loadBalancers,
		networkID:    networkID,
	}, nil
}

func (c *cloud) Initialize(clientBuilder cloudprovider.ControllerClientBuilder, stop <-chan struct{}) {
}

func readHrobotServers(hrobot hrobot.RobotClient) {
	go func() {
		for {
			servers, err := hrobot.ServerGetList()
			if err != nil {
				fmt.Fprintf(os.Stderr, "ERROR: get servers from hrobot: %v\n", err)
			}
			var hservers []HrobotServer
			for _, s := range servers {
				zone := strings.ToLower(strings.Split(s.Dc, "-")[0])
				server := HrobotServer{
					ID:     s.ServerNumber,
					Name:   s.ServerName,
					Type:   s.Product,
					Zone:   zone,
					Region: strings.ToLower(s.Dc),
					IP:     net.ParseIP(s.ServerIP),
				}
				hservers = append(hservers, server)
			}
			hrobotServers = hservers
			time.Sleep(time.Duration(hrobotPeriod) * time.Second)
		}
	}()
}

func (c *cloud) Instances() (cloudprovider.Instances, bool) {
	return c.instances, true
}

func (c *cloud) Zones() (cloudprovider.Zones, bool) {
	return c.zones, true
}

func (c *cloud) LoadBalancer() (cloudprovider.LoadBalancer, bool) {
	if c.loadBalancer == nil {
		return nil, false
	}
	return c.loadBalancer, true
}

func (c *cloud) Clusters() (cloudprovider.Clusters, bool) {
	return nil, false
}

func (c *cloud) Routes() (cloudprovider.Routes, bool) {
	return nil, false
}

func (c *cloud) ProviderName() string {
	return providerName
}

func (c *cloud) ScrubDNS(nameservers, searches []string) (nsOut, srchOut []string) {
	return nil, nil
}

func (c *cloud) HasClusterID() bool {
	return false
}

func init() {
	cloudprovider.RegisterCloudProvider(providerName, func(config io.Reader) (cloudprovider.Interface, error) {
		return newCloud(config)
	})
}
