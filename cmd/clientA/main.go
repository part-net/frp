// Copyright 2018 fatedier, fatedier@gmail.com
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"flag"
	"fmt"
	"os"
	"time"
)

var (
	cfgFlag       = flag.String("c", "", "config file of frpc")
	portFlag      = flag.Int("p", 50000, "Listening port for client A")
	intervalFlag  = flag.Duration("i", 3, "The interval between starting the frpc service and clientA, Unit: seconds")
	srvNameFlag   = flag.String("n", "client_b", "client b's server name")
	relayIpFlag   = flag.String("ri", "8.210.85.12", "relay server IP")
	relayPortFlag = flag.Int("rp", 7000, "relay server Port")
	stunSrvFlag   = flag.String("s", "8.210.40.156:3478", "stun server address")
)

func main() {
	// Do not show command usage here.
	// Parse and ensure all needed inputs are specified
	flag.Parse()
	cfgPath := *cfgFlag
	if cfgPath == "" {
		// 生成ini文件
		// frps configuration
		frpsCfg := &FrpsConfig{
			*relayIpFlag,
			*relayPortFlag,
			*stunSrvFlag,
			&AuthConfig{"token", true, true, "12345678_"},
		}
		err, filePath := genCfgFile(frpsCfg, *srvNameFlag, *portFlag)
		if err != nil {
			os.Exit(1)
		}
		cfgPath = filePath
	}

	// 启动frpc
	err := runClient(cfgPath)
	if err != nil {
		os.Exit(1)
	}

	time.Sleep((*intervalFlag) * time.Second)
	// 启动clientA
	serverAddr := fmt.Sprintf("127.0.0.1:%d", *portFlag)
	startClientA(serverAddr)
}
