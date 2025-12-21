package utils

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"

	"synapse-backend/internal/config"
	"synapse-backend/pkg/logger"

	"github.com/nacos-group/nacos-sdk-go/v2/clients"
	"github.com/nacos-group/nacos-sdk-go/v2/common/constant"
	"github.com/nacos-group/nacos-sdk-go/v2/vo"
)

// RegisterNacosService 注册服务到 Nacos
func RegisterNacosService() error {
	nacosConfig := config.AppConfig.Nacos
	if nacosConfig.ServerAddr == "" {
		return fmt.Errorf("Nacos 服务器地址为空")
	}

	// 解析 ServerAddr (host:port)
	parts := strings.Split(nacosConfig.ServerAddr, ":")
	if len(parts) != 2 {
		return fmt.Errorf("无效的 Nacos 服务器地址: %s", nacosConfig.ServerAddr)
	}
	host := parts[0]
	port, err := strconv.ParseUint(parts[1], 10, 64)
	if err != nil {
		return fmt.Errorf("无效的 Nacos 服务器端口: %v", err)
	}

	// 创建 ServerConfig
	sc := []constant.ServerConfig{
		*constant.NewServerConfig(host, port),
	}

	// 创建 ClientConfig
	cc := *constant.NewClientConfig(
		constant.WithNamespaceId(""), // public
		constant.WithTimeoutMs(5000),
		constant.WithNotLoadCacheAtStart(true),
		constant.WithLogDir("logs/nacos"),
		constant.WithCacheDir("logs/nacos/cache"),
		constant.WithLogLevel("debug"),
		constant.WithUsername(nacosConfig.Username),
		constant.WithPassword(nacosConfig.Password),
	)

	// 创建 Naming Client
	client, err := clients.NewNamingClient(
		vo.NacosClientParam{
			ClientConfig:  &cc,
			ServerConfigs: sc,
		},
	)
	if err != nil {
		return fmt.Errorf("创建 Nacos 命名客户端失败: %v", err)
	}

	// 获取本机 IP
	localIP := "172.27.16.1"
	
	// 注册服务
	success, err := client.RegisterInstance(vo.RegisterInstanceParam{
		Ip:          localIP,
		Port:        uint64(config.AppConfig.Server.Port),
		ServiceName: "synapse",
		GroupName:   "DEFAULT_GROUP",
		ClusterName: "DEFAULT",
		Weight:      10,
		Enable:      true,
		Healthy:     true,
		Ephemeral:   true,
		Metadata:    map[string]string{"version": "1.0.0"},
	})

	if !success || err != nil {
		// 如果是dev环境或未设置环境变量（默认dev），允许跳过Nacos注册
		env := os.Getenv("APP_ENV")
		if env == "" || env == "dev" {
			logger.Warnf("Dev环境或未设置环境，Nacos服务注册失败，跳过: %v", err)
			return nil
		}
		return fmt.Errorf("注册服务到 Nacos 失败: %v", err)
	}

	logger.Infof("服务注册到 Nacos: %s:%d", localIP, config.AppConfig.Server.Port)
	return nil
}

func getLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "127.0.0.1"
	}
	for _, address := range addrs {
		if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String()
			}
		}
	}
	return "127.0.0.1"
}
