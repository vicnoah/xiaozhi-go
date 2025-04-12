package ota

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"time"

	"github.com/sirupsen/logrus"
)

const (
	// DefaultOTAEndpoint 默认OTA服务器地址
	DefaultOTAEndpoint = "https://api.tenclass.net/xiaozhi/ota/"

	// 超时设置
	DefaultTimeout = 10 * time.Second
)

// ChipInfo 芯片信息结构
type ChipInfo struct {
	Model    int `json:"model"`
	Cores    int `json:"cores"`
	Revision int `json:"revision"`
	Features int `json:"features"`
}

// AppInfo 应用信息结构
type AppInfo struct {
	Name       string `json:"name"`
	Version    string `json:"version"`
	IDFVersion string `json:"idf_version"`
}

// BoardInfo 板信息结构
type BoardInfo struct {
	Type string `json:"type"`
	MAC  string `json:"mac"`
}

// OTAInfo OTA信息结构
type OTAInfo struct {
	Label string `json:"label"`
}

// DeviceInfo 设备信息结构，用于OTA请求
type DeviceInfo struct {
	FlashSize           int       `json:"flash_size"`
	MinimumFreeHeapSize int       `json:"minimum_free_heap_size"`
	MACAddress          string    `json:"mac_address"`
	ChipModelName       string    `json:"chip_model_name"`
	ChipInfo            ChipInfo  `json:"chip_info"`
	Application         AppInfo   `json:"application"`
	PartitionTable      []string  `json:"partition_table"`
	OTA                 OTAInfo   `json:"ota"`
	Board               BoardInfo `json:"board"`
}

// MQTTConfig MQTT配置结构
type MQTTConfig struct {
	Endpoint       string `json:"endpoint"`
	ClientID       string `json:"client_id"`
	PublishTopic   string `json:"publish_topic"`
	SubscribeTopic string `json:"subscribe_topic"`
}

// FirmwareInfo 固件信息结构
type FirmwareInfo struct {
	Version string `json:"version"`
}

// ActivationInfo 激活信息结构
type ActivationInfo struct {
	Code string `json:"code"`
}

// OTAResponse OTA响应结构
type OTAResponse struct {
	MQTT       MQTTConfig     `json:"mqtt"`
	Firmware   FirmwareInfo   `json:"firmware"`
	Activation ActivationInfo `json:"activation"`
}

// OTAClient OTA客户端结构
type OTAClient struct {
	Endpoint   string
	HTTPClient *http.Client
	DeviceInfo DeviceInfo
}

// NewOTAClient 创建新的OTA客户端
func NewOTAClient(deviceMAC, appVersion, boardType string) *OTAClient {
	// 创建HTTP客户端
	httpClient := &http.Client{
		Timeout: DefaultTimeout,
	}

	// 初始化设备信息
	deviceInfo := DeviceInfo{
		FlashSize:           16777216, // 16MB
		MinimumFreeHeapSize: 8318916,  // 8MB
		MACAddress:          deviceMAC,
		ChipModelName:       "generic",
		ChipInfo: ChipInfo{
			Model:    9,
			Cores:    runtime.NumCPU(),
			Revision: 2,
			Features: 18,
		},
		Application: AppInfo{
			Name:       "xiaozhi",
			Version:    appVersion,
			IDFVersion: "v5.3.2",
		},
		PartitionTable: []string{},
		OTA: OTAInfo{
			Label: "factory",
		},
		Board: BoardInfo{
			Type: boardType,
			MAC:  deviceMAC,
		},
	}

	return &OTAClient{
		Endpoint:   DefaultOTAEndpoint,
		HTTPClient: httpClient,
		DeviceInfo: deviceInfo,
	}
}

// RequestActivation 向服务器请求设备激活码
func (c *OTAClient) RequestActivation() (*OTAResponse, error) {
	// 将设备信息编码为JSON
	jsonData, err := json.Marshal(c.DeviceInfo)
	if err != nil {
		return nil, fmt.Errorf("编码设备信息失败: %v", err)
	}

	// 打印发送报文
	logrus.Debugf("发送请求体: %s", string(jsonData))

	// 创建HTTP请求
	req, err := http.NewRequest("POST", c.Endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("创建HTTP请求失败: %v", err)
	}

	// 设置请求头
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Device-Id", c.DeviceInfo.MACAddress)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "XiaoZhi-go/1.0")
	req.Header.Set("App-Version", c.DeviceInfo.Application.Version)
	req.Header.Set("Chip-Model", c.DeviceInfo.ChipModelName)
	req.Header.Set("Board-Type", c.DeviceInfo.Board.Type)

	// 打印请求头信息
	logrus.Debugf("请求URL: %s", req.URL.String())
	logrus.Debugf("请求头信息:")
	for key, values := range req.Header {
		for _, value := range values {
			logrus.Debugf("  %s: %s", key, value)
		}
	}

	// 发送请求
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("发送HTTP请求失败: %v", err)
	}
	defer resp.Body.Close()

	// 读取响应体
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应体失败: %v", err)
	}

	// 打印服务器应答
	logrus.Debugf("服务器状态码: %d", resp.StatusCode)
	logrus.Debugf("服务器响应头:")
	for key, values := range resp.Header {
		for _, value := range values {
			logrus.Debugf("  %s: %s", key, value)
		}
	}
	logrus.Debugf("服务器响应体: %s", string(body))

	// 检查响应状态码
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("服务器返回错误状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	// 解析响应JSON
	var otaResp OTAResponse
	if err := json.Unmarshal(body, &otaResp); err != nil {
		return nil, fmt.Errorf("解析服务器响应失败: %v", err)
	}

	if otaResp.Activation.Code == "" {
		logrus.Infof("设备已激活")
	} else {
		logrus.Infof("获取到设备激活码: %s", otaResp.Activation.Code)
	}
	return &otaResp, nil
}

// GetActivationCode 获取设备激活码
func (c *OTAClient) GetActivationCode() (string, error) {
	resp, err := c.RequestActivation()
	if err != nil {
		return "", err
	}

	return resp.Activation.Code, nil
}

// CheckFirmwareUpdate 检查固件更新
func (c *OTAClient) CheckFirmwareUpdate() (string, bool, error) {
	resp, err := c.RequestActivation()
	if err != nil {
		return "", false, err
	}

	currentVersion := c.DeviceInfo.Application.Version
	latestVersion := resp.Firmware.Version

	// 检查版本号是否相同
	if currentVersion == latestVersion {
		logrus.Infof("当前固件版本已是最新: %s", currentVersion)
		return latestVersion, false, nil
	}

	logrus.Infof("发现新版本固件: %s，当前版本: %s", latestVersion, currentVersion)
	return latestVersion, true, nil
}

// GetMQTTConfig 获取MQTT配置
func (c *OTAClient) GetMQTTConfig() (*MQTTConfig, error) {
	resp, err := c.RequestActivation()
	if err != nil {
		return nil, err
	}

	return &resp.MQTT, nil
}

// CheckActivationStatus 检查设备激活状态
func (c *OTAClient) CheckActivationStatus() (bool, error) {
	// 尝试获取激活信息
	resp, err := c.RequestActivation()
	if err != nil {
		return false, fmt.Errorf("获取激活信息失败: %v", err)
	}

	// 如果激活码为空，则认为已激活
	return resp.Activation.Code == "", nil
}
