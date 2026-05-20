import { Layout, Menu, Dropdown, Avatar, Space } from 'antd'
import {
  DashboardOutlined,
  CloudServerOutlined,
  ApiOutlined,
  FileSearchOutlined,
  SettingOutlined,
  QuestionCircleOutlined,
  UserOutlined,
  LogoutOutlined,
} from '@ant-design/icons'
import { Outlet, useLocation, useNavigate } from 'react-router-dom'
import { useAuth } from '../store/auth'
import { logout } from '../api/client'

const { Header, Sider, Content } = Layout

const items = [
  { key: '/', icon: <DashboardOutlined />, label: '总览' },
  { key: '/frps', icon: <CloudServerOutlined />, label: 'FRPS 节点' },
  { key: '/frpc', icon: <ApiOutlined />, label: 'FRPC 客户端' },
  { key: '/audit-logs', icon: <FileSearchOutlined />, label: '审计日志' },
  { key: '/settings', icon: <SettingOutlined />, label: '系统设置' },
  { key: '/help', icon: <QuestionCircleOutlined />, label: '使用说明' },
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
      <Sider breakpoint="lg" collapsedWidth="0">
        <div style={{ color: '#fff', padding: 16, fontWeight: 600, fontSize: 16 }}>service-edge</div>
        <Menu
          theme="dark"
          mode="inline"
          selectedKeys={[selected]}
          items={items}
          onClick={({ key }) => navigate(key)}
        />
      </Sider>
      <Layout>
        <Header style={{ background: '#fff', display: 'flex', justifyContent: 'flex-end', alignItems: 'center', paddingInline: 24 }}>
          <Dropdown
            menu={{
              items: [{ key: 'logout', icon: <LogoutOutlined />, label: '退出登录', onClick: onLogout }],
            }}
          >
            <Space style={{ cursor: 'pointer' }}>
              <Avatar size="small" icon={<UserOutlined />} />
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
