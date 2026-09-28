// 模型服务聚合根：provider 配置与加密后的凭据字段。
package domain

import "WorkBaby/internal/pkg"

var (
	ErrProviderNotFound = pkg.New(3101, "模型服务不存在", "")
	ErrNoProvider       = pkg.New(3102, "还没有配置模型服务", "去设置页添加一个")
	ErrProviderUnreach  = pkg.New(3103, "连不上这个模型服务", "检查地址与密钥")
)

// ProviderDO 模型服务配置。api_key 以 AES-GCM 密文存储，明文只在内存里出现。
type ProviderDO struct {
	ID        string `gorm:"primaryKey;size:64" json:"id"`
	Name      string `gorm:"size:128" json:"name"`
	API       string `gorm:"size:32" json:"api"`
	BaseURL   string `gorm:"size:512" json:"base_url"`
	APIKeyEnc string `gorm:"type:text" json:"-"`
	Models    string `gorm:"type:text" json:"-"`
	IsDefault bool   `json:"is_default"`
	Enabled   bool   `json:"enabled"`
	CreatedAt int64  `gorm:"autoCreateTime:milli" json:"created_at"`
	UpdatedAt int64  `gorm:"autoUpdateTime:milli" json:"updated_at"`
}

// TableName 显式指定表名：GORM 会把 DO 后缀复数化成 _dos。
func (ProviderDO) TableName() string { return "providers" }

// ProviderVO 出参：不返回密钥，只返回是否有密钥。
type ProviderVO struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	API       string   `json:"api"`
	BaseURL   string   `json:"base_url"`
	HasKey    bool     `json:"has_key"`
	Models    []string `json:"models"`
	IsDefault bool     `json:"is_default"`
	Enabled   bool     `json:"enabled"`
	CreatedAt int64    `json:"created_at"`
}

// UpsertProviderREQ 新增或更新模型服务。
type UpsertProviderREQ struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	API     string   `json:"api"`
	BaseURL string   `json:"base_url"`
	APIKey  string   `json:"api_key"`
	Models  []string `json:"models"`
}

// TestProviderREQ 连通测试入参。
type TestProviderREQ struct {
	ID string `json:"id"`
}

// TestProviderRESP 连通测试出参。
type TestProviderRESP struct {
	OK     bool   `json:"ok"`
	Model  string `json:"model"`
	Detail string `json:"detail"`
}
