package utils

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"synapse-backend/internal/config"
	"synapse-backend/pkg/logger"
)

var (
	once       sync.Once
	publicKey  string
	registered bool
)

type RegisterRequest struct {
	ServiceName string `json:"serviceName"`
	Endpoint    string `json:"endpoint"`   // 本服务对外地址
	PathPrefix  string `json:"pathPrefix"` // 本服务 API 前缀
}

type ChallengeResponse struct {
	Endpoint  string `json:"endpoint"` // 下一跳提交地址
	Nonce     string `json:"nonce"`
	Timestamp int64  `json:"timestamp"`
}

type ChallengeResponseRequest struct {
	ServiceName     string          `json:"serviceName"`
	Signature       string          `json:"signature"`
	OriginalRequest RegisterRequest `json:"originalRequest"`
}

type Result struct {
	Code int         `json:"code"`
	Msg  string      `json:"message"`
	Data interface{} `json:"data"`
}

// RegisterService 向 AuthService 注册本服务
func RegisterService() error {
	var err error
	once.Do(func() {
		err = register()
	})
	return err
}

func register() error {
	logger.Info("开始向 AuthService 注册服务")

	// Step 1: 请求 Challenge
	registerReq := RegisterRequest{
		ServiceName: "synapse", // 服务名
		Endpoint:    fmt.Sprintf("http://localhost:%d", config.AppConfig.Server.Port),
		PathPrefix:  "/api",
	}

	registerURL := config.AppConfig.Jwt.Upstream + "/service-registry/register"

	resp, err := http.Post(registerURL, "application/json", mustJSON(registerReq))
	if err != nil {
		return fmt.Errorf("请求 Challenge 失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("AuthService 返回非200状态码: %d", resp.StatusCode)
	}

	var result struct {
		Code int    `json:"code"`
		Msg  string `json:"message"`
		Data *ChallengeResponse
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("解析 Challenge 响应失败: %w", err)
	}

	if result.Code != 0 || result.Data == nil {
		return fmt.Errorf("获取 Challenge 失败: %s", result.Msg)
	}

	challenge := result.Data
	logger.Infof("收到 Challenge: nonce=%s, timestamp=%d", challenge.Nonce, challenge.Timestamp)

	// Step 2: 生成签名
	dataToSign := fmt.Sprintf("%s|%d", challenge.Nonce, challenge.Timestamp)
	signature := calculateHMAC(config.AppConfig.Jwt.SharedKey, dataToSign)

	// Step 3: 提交签名
	challengeReq := ChallengeResponseRequest{
		ServiceName:     registerReq.ServiceName,
		Signature:       signature,
		OriginalRequest: registerReq,
	}

	challengeURL := config.AppConfig.Jwt.Upstream + challenge.Endpoint

	resp, err = http.Post(challengeURL, "application/json", mustJSON(challengeReq))
	if err != nil {
		return fmt.Errorf("提交签名失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("提交签名返回非200状态码: %d", resp.StatusCode)
	}

	var finalResult Result
	if err := json.NewDecoder(resp.Body).Decode(&finalResult); err != nil {
		return fmt.Errorf("解析最终响应失败: %w", err)
	}

	if finalResult.Code != 0 {
		return fmt.Errorf("服务注册失败: %s", finalResult.Msg)
	}

	// 获取公钥
	pubKey, ok := finalResult.Data.(string)
	if !ok {
		return fmt.Errorf("公钥格式错误")
	}

	publicKey = pubKey
	registered = true

	logger.Infof("服务注册成功. 公钥已获取，长度: %d 字符", len(publicKey))
	return nil
}

// calculateHMAC 计算 HMAC-SHA256 签名
func calculateHMAC(key, data string) string {
	h := hmac.New(sha256.New, []byte(key))
	h.Write([]byte(data))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

func mustJSON(v interface{}) *strings.Reader {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return strings.NewReader(string(b))
}

// GetPublicKey 获取已注册的公钥
func GetPublicKey() (string, error) {
	if !registered {
		return "", fmt.Errorf("服务尚未注册")
	}
	return publicKey, nil
}
