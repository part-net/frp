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
	"bufio"
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/fatedier/frp/client"
	"github.com/fatedier/frp/pkg/config"
	"github.com/fatedier/frp/pkg/util/log"
)

// AuthConfig frp server authenticate struct:Enable token authentication
type AuthConfig struct {
	Method       string // authentication method
	HeartBeats   bool   // authenticate heartbeats
	NewWorkConns bool   // authenticate_new_work_conns
	Token        string // token authentication
}

// FrpsConfig frp server configuration structure
type FrpsConfig struct {
	ServerIP   string      // frp server ip
	ServerPort int         // frp server port
	StunServer string      // frp Stun Server
	Auth       *AuthConfig // Enable authentication mode
}

func handleTermSignal(svr *client.Service) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch
	svr.GracefulClose(500 * time.Millisecond)
}

func genCfgFile(frps *FrpsConfig, srvName string, clientPort int) (error, string) {
	currentDir, err := os.Getwd()
	if err != nil {
		fmt.Println("无法获取当前目录:", err)
		return err, ""
	}
	fileName := "config.ini"
	filePath := filepath.Join(currentDir, fileName)
	file, err := os.Create(filePath)
	defer file.Close()
	if err != nil {
		log.Error("failed to create Frps config file:", err)
		return err, filePath
	}
	// 创建一个写入器
	writer := bufio.NewWriter(file)

	// Write the frp server configuration
	fmt.Fprintln(writer, "[common]")
	fmt.Fprintln(writer, "server_addr =", frps.ServerIP)
	fmt.Fprintln(writer, "server_port =", frps.ServerPort)
	if "" != frps.StunServer {
		fmt.Fprintln(writer, "nat_hole_stun_server=", frps.StunServer)
	}
	if nil != frps.Auth {
		fmt.Fprintln(writer, "authentication_method =", frps.Auth.Method)
		fmt.Fprintln(writer, "authenticate_heartbeats =", frps.Auth.HeartBeats)
		fmt.Fprintln(writer, "authenticate_new_work_conns =", frps.Auth.NewWorkConns)
		fmt.Fprintln(writer, "token =", frps.Auth.Token)
	}
	fmt.Fprintln(writer, "")

	// 写入对端clientB内容到文件
	visitorTag := "[" + srvName + "_visitor]"
	fmt.Fprintln(writer, visitorTag)
	fmt.Fprintln(writer, "type = xtcp")
	fmt.Fprintln(writer, "role = visitor")
	fmt.Fprintln(writer, "server_name =", srvName)
	fmt.Fprintln(writer, "sk = abcdefg")
	fmt.Fprintln(writer, "bind_addr = 127.0.0.1")
	fmt.Fprintln(writer, "bind_port =", clientPort)
	fmt.Fprintln(writer, "")

	// 刷新缓冲并将数据写入文件
	err = writer.Flush()
	if err != nil {
		fmt.Println("Error flushing buffer:", err)
	}

	return err, filePath
}

func runClient(cfgFilePath string) error {
	cfg, pxyCfgs, visitorCfgs, err := config.ParseClientConfig(cfgFilePath)
	if err != nil {
		fmt.Println(err)
		return err
	}
	return startService(cfg, pxyCfgs, visitorCfgs, cfgFilePath)
}

func startService(
	cfg config.ClientCommonConf,
	pxyCfgs map[string]config.ProxyConf,
	visitorCfgs map[string]config.VisitorConf,
	cfgFile string,
) (err error) {
	log.InitLog(cfg.LogWay, cfg.LogFile, cfg.LogLevel,
		cfg.LogMaxDays, cfg.DisableLogColor)

	if cfgFile != "" {
		log.Info("start frpc service for config file [%s]", cfgFile)
		defer log.Info("frpc service for config file [%s] stopped", cfgFile)
	}
	svr, errRet := client.NewService(cfg, pxyCfgs, visitorCfgs, cfgFile)
	if errRet != nil {
		err = errRet
		return
	}

	shouldGracefulClose := cfg.Protocol == "kcp" || cfg.Protocol == "quic"
	// Capture the exit signal if we use kcp or quic.
	if shouldGracefulClose {
		go handleTermSignal(svr)
	}

	go svr.Run(context.Background())
	return
}

// 不指定客户端端口
func startClientA(serverAddr string) {
	// serverAddr := "127.0.0.1:50000"
	fmt.Println("start clientA.......")
	// 连接到服务器
	conn, err := net.Dial("tcp", serverAddr)
	if err != nil {
		fmt.Println("Failed to connect the server:", err)
		return
	}
	defer conn.Close()

	fmt.Println("Successfully connected to the server:", serverAddr)

	// 启动一个goroutine用于接收来自服务器的消息并显示在屏幕上
	go receiveMessages(conn)

	// 发送一条hello world消息
	_, err = conn.Write([]byte("hello world" + "\n"))
	if err != nil {
		fmt.Println("Failed to send the hello world message:", err)
		return
	} else {
		fmt.Println("Congratulations on the success of the hole and the completion of the hello world message...")
	}

	// 从标准输入获取用户输入并发送到服务器
	fmt.Println("Continue sending messages if needed, Please enter a message (enter 'exit' to exit):")
	reader := bufio.NewReader(os.Stdin)
	for {
		message, _ := reader.ReadString('\n')
		message = strings.TrimSpace(message)
		if message == "exit" {
			fmt.Println("Exit the client")
			return
		}
		_, err := conn.Write([]byte(message + "\n"))
		if err != nil {
			fmt.Println("Regret failure to impress Failure to send the message:", err)
			return
		} else {
			fmt.Println("Message sent successfully:", message)
		}
	}
}

// 接收来自服务器的消息并显示在屏幕上
func receiveMessages(conn net.Conn) {
	reader := bufio.NewReader(conn)
	for {
		message, err := reader.ReadString('\n')
		if err != nil {
			// fmt.Println("Unable to read the entered message:", err)
			// return
			continue
		}
		fmt.Printf("receive from: [%s], the message: %s\n", conn.RemoteAddr(), message)
	}
}
