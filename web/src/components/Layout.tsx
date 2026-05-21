import { Breadcrumb, Layout, Menu, Dropdown, Avatar, Space } from 'antd'
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

const menuItems = [
  { key: '/', icon: <DashboardOutlined />, label: '总览' },
  { key: '/topology', icon: <PartitionOutlined />, label: '网络拓扑' },
  { key: '/frps', icon: <CloudServerOutlined />, label: 'FRPS 节点' },
  { key: '/frpc', icon: <ApiOutlined />, label: 'FRPC 主机' },
  { key: '/audit-logs', icon: <FileSearchOutlined />, label: '审计日志' },
  { key: '/settings', icon: <SettingOutlined />, label: '系统设置' },
  { key: '/help', icon: <QuestionCircleOutlined />, label: '使用说明' },
  { key: '/about', icon: <InfoCircleOutlined />, label: '关于' },
]

const menuKeys = menuItems.map((i) => i.key)

// Menu entries that act as parent containers for sub-routes.
const parentPrefixes: { prefix: string; menuKey: string }[] = [
  { prefix: '/frps', menuKey: '/frps' },
  { prefix: '/frpc', menuKey: '/frpc' },
  { prefix: '/connections', menuKey: '/frpc' },
]

function resolveSelectedKey(pathname: string): string {
  if (menuKeys.includes(pathname)) return pathname
  for (const { prefix, menuKey } of parentPrefixes) {
    if (pathname.startsWith(prefix)) return menuKey
  }
  return '/'
}

function resolvePageTitle(pathname: string): string {
  if (pathname === '/frps') return 'FRPS 节点'
  if (pathname === '/frps/new') return '新建 FRPS 节点'
  if (pathname.startsWith('/frps/')) return 'FRPS 节点详情'
  if (pathname === '/frpc') return 'FRPC 主机'
  if (pathname === '/frpc/new') return '新建 FRPC 主机'
  if (pathname.match(/^\/frpc\/[^/]+\/connections\/new$/)) return '新增连接'
  if (pathname.startsWith('/frpc/')) return 'FRPC 主机详情'
  if (pathname.startsWith('/connections/')) return '连接详情'
  if (pathname === '/topology') return '网络拓扑'
  if (pathname === '/audit-logs') return '审计日志'
  if (pathname === '/settings') return '系统设置'
  if (pathname === '/help') return '使用说明'
  if (pathname === '/about') return '关于'
  return '总览'
}

interface Crumb {
  title: string
  path?: string
}

function resolveBreadcrumbs(pathname: string): Crumb[] {
  if (pathname === '/') return [{ title: '总览' }]

  if (pathname === '/topology') return [{ title: '总览', path: '/' }, { title: '网络拓扑' }]
  if (pathname === '/audit-logs') return [{ title: '总览', path: '/' }, { title: '审计日志' }]
  if (pathname === '/settings') return [{ title: '总览', path: '/' }, { title: '系统设置' }]
  if (pathname === '/help') return [{ title: '总览', path: '/' }, { title: '使用说明' }]
  if (pathname === '/about') return [{ title: '总览', path: '/' }, { title: '关于' }]

  if (pathname.startsWith('/frps')) {
    if (pathname === '/frps') return [{ title: 'FRPS 节点' }]
    if (pathname === '/frps/new') return [{ title: 'FRPS 节点', path: '/frps' }, { title: '新建' }]
    return [{ title: 'FRPS 节点', path: '/frps' }, { title: '详情' }]
  }

  if (pathname.startsWith('/frpc')) {
    if (pathname === '/frpc') return [{ title: 'FRPC 主机' }]
    if (pathname.match(/^\/frpc\/[^/]+\/connections\/new$/)) {
      return [{ title: 'FRPC 主机', path: '/frpc' }, { title: '新增连接' }]
    }
    return [{ title: 'FRPC 主机', path: '/frpc' }, { title: '详情' }]
  }

  if (pathname.startsWith('/connections/')) {
    return [{ title: '连接详情' }]
  }

  return [{ title: '总览' }]
}

export default function AppLayout() {
  const navigate = useNavigate()
  const location = useLocation()
  const { user, clear } = useAuth()

  const selectedKey = resolveSelectedKey(location.pathname)
  const pageTitle = resolvePageTitle(location.pathname)
  const breadcrumbItems = resolveBreadcrumbs(location.pathname)

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
          <span style={{ width: 16, height: 18, background: '#024ad8', transform: 'skewX(-12deg)', flex: '0 0 auto' }} />
          <div style={{ lineHeight: 1.25 }}>
            <div style={{ color: '#fff', fontWeight: 700, fontSize: 14 }}>云梦镜像边缘服务网络</div>
            <div style={{ fontSize: 11, color: 'rgba(255,255,255,0.45)', letterSpacing: 1 }}>service-edge</div>
          </div>
        </div>
        <Menu
          theme="dark"
          mode="inline"
          selectedKeys={[selectedKey]}
          items={menuItems}
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
          <div>
            <div style={{ fontSize: 16, fontWeight: 600, color: '#1a1a1a', marginBottom: breadcrumbItems.length > 1 ? 2 : 0 }}>
              {pageTitle}
            </div>
            {breadcrumbItems.length > 1 && (
              <Breadcrumb
                items={breadcrumbItems.map((c) => ({
                  title: c.path ? (
                    <a onClick={() => navigate(c.path!)} style={{ fontSize: 12 }}>{c.title}</a>
                  ) : (
                    <span style={{ fontSize: 12 }}>{c.title}</span>
                  ),
                }))}
              />
            )}
          </div>
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
