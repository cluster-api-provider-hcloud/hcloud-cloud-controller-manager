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
	"os"
	"strconv"
	"strings"

	"github.com/hetznercloud/hcloud-go/hcloud"
	cloudprovider "k8s.io/cloud-provider"
)

func getServerByName(ctx context.Context, c commonClient, name string) (server *hcloud.Server, err error) {
	const op = "hcloud/getServerByName"

	server, _, err = c.Hcloud.Server.GetByName(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	if server == nil {
		// try hrobot find
		server, err = hrobotGetServerByName(name)
		if server == nil {
			fmt.Fprintf(os.Stderr, "ERROR: Not found serverName: %v, in hcloud and hrobot\n", name)
			return nil, cloudprovider.InstanceNotFound
		}
	}
	return server, nil
}

func getServerByID(ctx context.Context, c commonClient, id int) (*hcloud.Server, error) {
	const op = "hcloud/getServerByName"

	server, _, err := c.Hcloud.Server.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	if server == nil {

		// try hrobot find
		server, err = hrobotGetServerByID(id)
		if server == nil {
			fmt.Fprintf(os.Stderr, "ERROR: Not found serverID: %v, in hcloud and hrobot\n", id)
			return nil, cloudprovider.InstanceNotFound
		}
	}
	return server, nil
}

func providerIDToServerID(providerID string) (int, error) {
	const op = "hcloud/providerIDToServerID"

	providerPrefix := providerName + "://"
	if !strings.HasPrefix(providerID, providerPrefix) {
		return 0, fmt.Errorf("%s: missing prefix hcloud://: %s", op, providerID)
	}

	idString := strings.ReplaceAll(providerID, providerPrefix, "")
	if idString == "" {
		return 0, fmt.Errorf("%s: missing serverID: %s", op, providerID)
	}

	id, err := strconv.Atoi(idString)
	if err != nil {
		return 0, fmt.Errorf("%s: invalid serverID: %s", op, providerID)
	}
	return id, nil
}

func hrobotGetServerByName(name string) (*hcloud.Server, error) {
	for _, s := range hrobotServers {
		if s.Name == name {
			server := &hcloud.Server{
				ID:         s.ID,
				Name:       s.Name,
				PublicNet:  hcloud.ServerPublicNet{IPv4: hcloud.ServerPublicNetIPv4{IP: s.IP}},
				ServerType: &hcloud.ServerType{Name: s.Type},
				Status:     hcloud.ServerStatus("running"),
				Datacenter: &hcloud.Datacenter{Location: &hcloud.Location{Name: s.Zone}, Name: s.Region},
			}
			return server, nil
		}
	}
	// server not found
	return nil, nil
}

func hrobotGetServerByID(id int) (*hcloud.Server, error) {
	for _, s := range hrobotServers {
		if s.ID == id {
			server := &hcloud.Server{
				ID:         s.ID,
				Name:       s.Name,
				PublicNet:  hcloud.ServerPublicNet{IPv4: hcloud.ServerPublicNetIPv4{IP: s.IP}},
				ServerType: &hcloud.ServerType{Name: s.Type},
				Status:     hcloud.ServerStatus("running"),
				Datacenter: &hcloud.Datacenter{Location: &hcloud.Location{
					Name:        s.Zone,
					NetworkZone: hcloud.NetworkZoneEUCentral},
					Name: s.Region},
			}
			return server, nil
		}
	}
	// server not found
	return nil, nil
}
