package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	pan123 "github.com/okatu-loli/go-123pan"
)

// config 是保存在配置目录的凭证。
type config struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

// tokenCache 是跨进程复用的 access_token 缓存。
type tokenCache struct {
	AccessToken string    `json:"access_token"`
	ExpiredAt   time.Time `json:"expired_at"`
}

func configDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "pan123")
	return dir, os.MkdirAll(dir, 0o700)
}

func configPath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

func tokenPath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "token.json"), nil
}

func saveConfig(c *config) error {
	p, err := configPath()
	if err != nil {
		return err
	}
	data, _ := json.MarshalIndent(c, "", "  ")
	return os.WriteFile(p, data, 0o600)
}

func loadConfig() (*config, error) {
	// 环境变量优先
	if id, secret := os.Getenv("PAN123_CLIENT_ID"), os.Getenv("PAN123_CLIENT_SECRET"); id != "" && secret != "" {
		return &config{ClientID: id, ClientSecret: secret}, nil
	}
	p, err := configPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errors.New("未找到凭证，请先运行: pan123 login -client-id ID -client-secret SECRET\n（或设置环境变量 PAN123_CLIENT_ID / PAN123_CLIENT_SECRET）")
		}
		return nil, err
	}
	var c config
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("配置文件损坏 (%s): %w", p, err)
	}
	return &c, nil
}

func removeConfig() error {
	for _, fn := range []func() (string, error){configPath, tokenPath} {
		p, err := fn()
		if err != nil {
			return err
		}
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func saveToken(token string, expiredAt time.Time) error {
	p, err := tokenPath()
	if err != nil {
		return err
	}
	data, _ := json.Marshal(&tokenCache{AccessToken: token, ExpiredAt: expiredAt})
	return os.WriteFile(p, data, 0o600)
}

func loadToken() *tokenCache {
	p, err := tokenPath()
	if err != nil {
		return nil
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return nil
	}
	var t tokenCache
	if err := json.Unmarshal(data, &t); err != nil {
		return nil
	}
	return &t
}

// newClient 加载凭证与缓存 token，构造 SDK 客户端。
func newClient() (*pan123.Client, error) {
	cfg, err := loadConfig()
	if err != nil {
		return nil, err
	}
	c := pan123.New(cfg.ClientID, cfg.ClientSecret)
	if t := loadToken(); t != nil && time.Until(t.ExpiredAt) > 15*time.Minute {
		c.SetToken(t.AccessToken, t.ExpiredAt)
	}
	return c, nil
}

// persistToken 把（可能已被 SDK 自动刷新的）token 写回缓存文件。
func persistToken(c *pan123.Client) {
	token, expiredAt := c.TokenInfo()
	if token != "" && !expiredAt.IsZero() {
		_ = saveToken(token, expiredAt)
	}
}
