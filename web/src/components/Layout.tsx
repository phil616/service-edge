import { Layout, Menu, Dropdown, Avatar, Space } from 'antd'
import {
  DashboardOutlined,
  CloudServerOutlined,
  ApiOutlined,
  FileSearchOutlined,
  SettingOutlined,
  QuestionCircleOutlined,
  PartitionOutlined,
  InfoCircleOutlined,
  UserOutlined,
  LogoutOutlined,
} from '@ant-design/icons'
import { Outlet, useLocation, useNavigate } from 'react-router-dom'
import { useAuth } from '../store/auth'
import { logout } from '../api/client'

const { Header, Sider, Content } = Layout

const items = [
  { key: '/', icon: <DashboardOutlined />, label: '总览' },
  { key: '/topology', icon: <PartitionOutlined />, label: '网络拓扑' },
  { key: '/frps', icon: <CloudServerOutlined />, label: 'FRPS 节点' },
  { key: '/frpc', icon: <ApiOutlined />, label: 'FRPC 客户端' },
  { key: '/audit-logs', icon: <FileSearchOutlined />, label: '审计日志' },
  { key: '/settings', icon: <SettingOutlined />, label: '系统设置' },
  { key: '/help', icon: <QuestionCircleOutlined />, label: '使用说明' },
  { key: '/about', icon: <InfoCircleOutlined />, label: '关于' },
]

export default function AppLayout() {
  const navigate = useNavigate()
  const location = useLocation()
  const { user, clear } = useAuth()

  const selected =
    items
      .map((i) => i.key)
      .filter((k) => (k === '/' ? location.pathname === '/' : location.pathname.startsWith(k)))
      .sort((a, b) => b.length - a.length)[0] || '/'

  const activeLabel = items.find((i) => i.key === selected)?.label ?? ''

  const onLogout = async () => {
    try {
      await logout()
    } catch {
      /* ignore */
    }
    clear()
    navigate('/login')
  }

  return (
    <Layout style={{ minHeight: '100vh' }}>
      <Sider breakpoint="lg" collapsedWidth="0" width={232}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 10, padding: '18px 16px 14px' }}>
          {/* Wordmark accent — a single skewed electric-blue slash echoing the brand. */}
          <span style={{ width: 16, height: 18, background: '#024ad8', transform: 'skewX(-12deg)', flex: '0 0 auto' }} />
          <div style={{ lineHeight: 1.25 }}>
            <div style={{ color: '#fff', fontWeight: 700, fontSize: 14 }}>云梦镜像边缘服务网络</div>
            <div style={{ fontSize: 11, color: 'rgba(255,255,255,0.45)', letterSpacing: 1 }}>service-edge</div>
          </div>
        </div>
        <Menu
          theme="dark"
          mode="inline"
          selectedKeys={[selected]}
          items={items}
          onClick={({ key }) => navigate(key)}
          style={{ borderInlineEnd: 'none' }}
        />
      </Sider>
      <Layout>
        <Header
          style={{
            background: '#fff',
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'center',
            paddingInline: 24,
            borderBottom: '1px solid #e8e8e8',
          }}
        >
          <span style={{ fontSize: 16, fontWeight: 600, color: '#1a1a1a' }}>{activeLabel}</span>
          <Dropdown
            menu={{
              items: [{ key: 'logout', icon: <LogoutOutlined />, label: '退出登录', onClick: onLogout }],
            }}
          >
            <Space style={{ cursor: 'pointer' }}>
              <Avatar size="small" style={{ background: '#024ad8' }} icon={<UserOutlined />} />
              {user?.username}
            </Space>
          </Dropdown>
        </Header>
        <Content style={{ margin: 24 }}>
          <Outlet />
        </Content>
      </Layout>
    </Layout>
  )
}
