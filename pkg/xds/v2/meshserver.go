/*
 * Licensed to the Apache Software Foundation (ASF) under one or more
 * contributor license agreements.  See the NOTICE file distributed with
 * this work for additional information regarding copyright ownership.
 * The ASF licenses this file to You under the Apache License, Version 2.0
 * (the "License"); you may not use this file except in compliance with
 * the License.  You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

/* Package xds can be used to create an grpc (just support grpc and v2 api) client communicated with pilot
   and fetch config in cycle
*/

package v2

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"

	"mosn.io/mosn/pkg/log"
	"mosn.io/mosn/pkg/types"
)

type MeshBody struct {
	Code    int      `json:"code"`
	Message *MeshMsg `json:"message"`
}

type MeshMsg struct {
	PilotMsg *PilotSharding `json:"pilotSharding"`
	Labels   *LabelConfig   `json:"labels"`
}

type PilotSharding struct {
	Vips      []PilotAddrConfig `json:"vips"`
	Namespace string            `json:"namespace"`
}

type PilotAddrConfig struct {
	Ip         string `json:"ip"`
	Port       int    `json:"port"`
	targetPort int    `json:"targetPort"`
}

type LabelConfig struct {
	Tenant    string `json:"tenant"`
	Cluster   string `json:"cluster"`
	Workspace string `json:"workspace"`
}

var (
	RespFmtErr    = errors.New("mesh server response format error")
	NoClusterName = errors.New("cluster name not found")
	NoServiceNode = errors.New("no service node found")
)

var (
	MeshServerAddr string
	MeshServerApi  = "/apis/v1/mesh/pilot/"
	ClusterEnv = "RD_APP_CLUSTER"
)

func SetMeshServerConfig(addr string, uri string) {
	MeshServerAddr = addr
	MeshServerApi = uri
}

// connect to mesh server and get pilot addressed
func (c *ClusterConfig) UpdateFromMeshServer() error {
	cluster := os.Getenv(ClusterEnv)
	if cluster == "" {
		if cluster = strings.Split(os.Getenv("HOSTNAME"), ".")[1]; cluster == "" {
			return NoClusterName
		}
	}
	uri := fmt.Sprintf("http://%s%s%s", MeshServerAddr, MeshServerApi, cluster)

	resp, err := http.Get(uri)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	b, err := ioutil.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return errors.New(fmt.Sprintf("invalid response code from mesh server: %d, uri: %s ", resp.StatusCode, uri))
	}

	meshConf := MeshBody{}
	err = json.Unmarshal(b, &meshConf)
	if err != nil {
		return err
	}
	msg := meshConf.Message
	if msg == nil || msg.PilotMsg == nil {
		return RespFmtErr
	}

	newAddr := []string{}
	for _, addr := range msg.PilotMsg.Vips {
		newAddr = append(newAddr, fmt.Sprintf("%s:%d", addr.Ip, addr.Port))
	}
	c.Address = newAddr
	log.DefaultLogger.Infof("get pilot add from mesh server, addr: %v", newAddr)

	// update service Node
	if msg.Labels == nil {
		return nil
	}
	nodeInfo := fmt.Sprintf("multitenancy.tenant=%s~multitenancy.workspace=%s~multitenancy.cluster=%s",
		msg.Labels.Tenant, msg.Labels.Workspace, msg.Labels.Cluster)
	sn := strings.Split(types.GetGlobalXdsInfo().ServiceNode, "||")
	if len(sn) < 2 {
		return NoServiceNode
	}
	types.GetGlobalXdsInfo().ServiceNode = sn[0] + "||" + nodeInfo

	return nil
}
