package model

import "time"

// User is a control-plane login user.
type User struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	Username     string    `gorm:"uniqueIndex;not null" json:"username"`
	PasswordHash string    `gorm:"not null" json:"-"`
	CreatedAt    time.Time `json:"created_at"`
}

// FRPSNode is a public edge node running frps.
type FRPSNode struct {
	ID            uint       `gorm:"primaryKey" json:"id"`
	UUID          string     `gorm:"column:uuid;uniqueIndex;not null" json:"uuid"`
	Name          string     `gorm:"not null" json:"name"`
	BindPort      int        `gorm:"column:bind_port;not null" json:"bind_port"`
	DashboardPort *int       `gorm:"column:dashboard_port" json:"dashboard_port,omitempty"`
	DashboardUser string     `gorm:"column:dashboard_user" json:"dashboard_user,omitempty"`
	DashboardPwd  string     `gorm:"column:dashboard_pwd" json:"-"`
	FrpToken      string     `gorm:"column:frp_token;not null" json:"-"`
	TLSCert       string     `gorm:"column:tls_cert;not null" json:"-"`
	TLSKey        string     `gorm:"column:tls_key;not null" json:"-"`
	FrpVersion    string     `gorm:"column:frp_version;not null" json:"frp_version"`
	ConfigVersion int        `gorm:"column:config_version;default:1" json:"config_version"`
	Status        string     `gorm:"default:pending" json:"status"`
	LastHeartbeat *time.Time `gorm:"column:last_heartbeat" json:"last_heartbeat,omitempty"`
	PublicIP      string     `gorm:"column:public_ip" json:"public_ip,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// FRPCClient is an internal client running an frpc instance.
type FRPCClient struct {
	ID            uint       `gorm:"primaryKey" json:"id"`
	UUID          string     `gorm:"column:uuid;uniqueIndex;not null" json:"uuid"`
	Name          string     `gorm:"not null" json:"name"`
	FRPSUUID      string     `gorm:"column:frps_uuid;index;not null" json:"frps_uuid"`
	TLSCert       string     `gorm:"column:tls_cert;not null" json:"-"`
	TLSKey        string     `gorm:"column:tls_key;not null" json:"-"`
	FrpVersion    string     `gorm:"column:frp_version;not null" json:"frp_version"`
	ConfigVersion int        `gorm:"column:config_version;default:1" json:"config_version"`
	Status        string     `gorm:"default:pending" json:"status"`
	LastHeartbeat *time.Time `gorm:"column:last_heartbeat" json:"last_heartbeat,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`

	Proxies []ProxyMapping `gorm:"-" json:"proxies,omitempty"`
}

// ProxyMapping is one port mapping belonging to an frpc client.
type ProxyMapping struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	FRPCUUID      string    `gorm:"column:frpc_uuid;index;not null" json:"frpc_uuid"`
	Name          string    `gorm:"not null" json:"name"`
	ProxyType     string    `gorm:"column:proxy_type;not null" json:"proxy_type"` // tcp/udp/http/https
	LocalIP       string    `gorm:"column:local_ip;default:127.0.0.1" json:"local_ip"`
	LocalPort     int       `gorm:"column:local_port;not null" json:"local_port"`
	RemotePort    *int      `gorm:"column:remote_port" json:"remote_port,omitempty"`
	CustomDomains string    `gorm:"column:custom_domains" json:"custom_domains,omitempty"` // JSON array
	Subdomain     string    `json:"subdomain,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

// EnrollmentToken is a one-time install token.
type EnrollmentToken struct {
	Token      string     `gorm:"column:token;primaryKey" json:"token"`
	TargetType string     `gorm:"column:target_type;not null" json:"target_type"` // frps/frpc
	TargetUUID string     `gorm:"column:target_uuid;not null" json:"target_uuid"`
	ExpiresAt  time.Time  `gorm:"column:expires_at;index;not null" json:"expires_at"`
	UsedAt     *time.Time `gorm:"column:used_at" json:"used_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

// AuditLog records a write operation.
type AuditLog struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	UserID     *uint     `gorm:"column:user_id" json:"user_id,omitempty"`
	Action     string    `gorm:"not null" json:"action"`
	TargetType string    `gorm:"column:target_type" json:"target_type,omitempty"`
	TargetUUID string    `gorm:"column:target_uuid" json:"target_uuid,omitempty"`
	Detail     string    `json:"detail,omitempty"` // JSON
	IP         string    `gorm:"column:ip" json:"ip,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

// AllModels returns every model for auto-migration.
func AllModels() []any {
	return []any{
		&User{},
		&FRPSNode{},
		&FRPCClient{},
		&ProxyMapping{},
		&EnrollmentToken{},
		&AuditLog{},
	}
}
