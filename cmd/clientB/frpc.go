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
	fmt.Fprintln(writer, "log_file = frpc.log")
	fmt.Fprintln(writer, "")

	// 写入对端clientB内容到文件
	srvTagName := "[" + srvName + "]"
	fmt.Fprintln(writer, srvTagName)
	fmt.Fprintln(writer, "type = xtcp")
	fmt.Fprintln(writer, "sk = abcdefg")
	fmt.Fprintln(writer, "local_ip = 127.0.0.1")
	fmt.Fprintln(writer, "local_port =", clientPort)
	fmt.Fprintln(writer, "use_encryption = false")
	fmt.Fprintln(writer, "use_compression = false")
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

func startClientB(addr string) {
	// 监听地址和端口
	// addr := "0.0.0.0:20000"

	// 创建TCP监听配置
	config := net.ListenConfig{
		Control: func(network, address string, c syscall.RawConn) error {
			return c.Control(func(fd uintptr) {
				// 设置SO_REUSEADDR选项
				err := syscall.SetsockoptInt(syscall.Handle(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
				if err != nil {
					fmt.Println("Failed to set the SO REUSEADDR option:", err)
				}
			})
		},
	}

	// 创建TCP监听器
	listener, err := config.Listen(context.Background(), "tcp", addr)
	if err != nil {
		fmt.Println("Failed to create TCP listener:", err)
		return
	}
	defer listener.Close()

	fmt.Printf("TCP server started:[%v], waiting for connection...", addr)

	// 设置信号处理，捕获Ctrl+C信号以优雅地关闭服务器
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		fmt.Println("The termination signal is received and the server is shut down...")
		listener.Close()
		os.Exit(0)
	}()

	for {
		// 接受客户端连接
		conn, err := listener.Accept()
		if err != nil {
			fmt.Println("Accept connection failure:", err)
			continue
		}

		// 在新的goroutine中处理连接
		go handleConnection(conn)
	}
}

func handleConnection(conn net.Conn) {
	defer conn.Close()

	fmt.Printf("client:[ %s ] was connected.\n", conn.RemoteAddr())

	buffer := make([]byte, 1024)

	for {
		// 读取客户端发送的数据
		n, err := conn.Read(buffer)
		if err != nil {
			fmt.Printf("client:[ %s ] was disconnected.\n", conn.RemoteAddr())
			return
		}

		// 处理客户端发送的数据，这里简单地回显
		data := buffer[:n]
		fmt.Printf("message received from:[ %s ], message:%s", conn.RemoteAddr(), string(data))

		// 回复客户端
		message := fmt.Sprintf("Client B has received the data:%s", string(data))
		_, err = conn.Write([]byte(message))
		if err != nil {
			fmt.Println("Failed to reply to client:", err)
			return
		}
	}
}
