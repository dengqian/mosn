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
	"mosn.io/mosn/pkg/types"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
)

var resp = `
	{
    	"code": 0,
    	"message": {
    	    "labels": {
    	        "cluster": "test-cdn-k8s-cn-shenzhen-1",
    	        "tenant": "cdn",
    	        "workspace": "test"
    	    },
    	    "pilotSharding": {
    	        "namespace": "pilot-sharding-group-0",
    	        "vips": [
    	            {
    	                "ip": "140.205.221.5",
    	                "port": 8001,
    	                "targetPort": 30002
    	            }
    	        ]
    	    }
    	}
	}`

func Test_UnmarshalConfig(t *testing.T) {
	msg := MeshBody{}
	err := json.Unmarshal([]byte(resp), &msg)
	if err != nil || msg.Message == nil || msg.Message.PilotMsg == nil || msg.Message.Labels == nil {
		t.Fatalf("mesh server response parse error:%v", err)
	}
}

func handler1(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(200)
	w.Write([]byte(resp))
}

func Test_ConnectMeshServer(t *testing.T) {
	mux1 := http.NewServeMux()
	mux1.HandleFunc("/apis/v1/mesh/pilot/test", handler1)
	s := &http.Server{
		Addr:    "127.0.0.1:13477",
		Handler: mux1,
	}
	ln, err := net.Listen("tcp", s.Addr)
	if err != nil {
		t.Errorf("TestService net.Listern error: %v", err)
	}
	go s.Serve(ln)

	os.Setenv(ClusterEnv, "test")
	SetMeshServerConfig("127.0.0.1:13477", "/apis/v1/mesh/pilot/")
	types.GetGlobalXdsInfo().ServiceNode = "a.svc.cluster.local||multitenancy.tenant=cdn~multitenancy.workspace=test~multitency.cluster=test"
	c := ClusterConfig{}
	if err = c.UpdateFromMeshServer(); err != nil {
		t.Errorf("update from mesh server failed")
	}

	if strings.Contains(types.GetGlobalXdsInfo().ServiceNode, "shenzhen") != true {
		t.Errorf("update service node failed")
	}
}
