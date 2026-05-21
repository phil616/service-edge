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

interface Crumb {
  title: string
  path?: string
}

// resolveNav is the single source of truth for the header: it returns the page
// title together with the full breadcrumb trail whose LAST crumb always equals
// the title, so the two can never diverge. The home crumb is included on every
// non-root page; the leaf crumb is non-clickable (the current page).
function resolveNav(pathname: string): { title: string; crumbs: Crumb[] } {
  const home: Crumb = { title: '总览', path: '/' }
  const trail = (title: string, ...mid: Crumb[]): { title: string; crumbs: Crumb[] } => ({
    title,
    crumbs: [home, ...mid, { title }],
  })

  if (pathname === '/') return { title: '总览', crumbs: [] }
  if (pathname === '/topology') return trail('网络拓扑')
  if (pathname === '/audit-logs') return trail('审计日志')
  if (pathname === '/settings') return trail('系统设置')
  if (pathname === '/help') return trail('使用说明')
  if (pathname === '/about') return trail('关于')

  if (pathname === '/frps') return trail('FRPS 节点')
  if (pathname === '/frps/new') return trail('新建 FRPS 节点', { title: 'FRPS 节点', path: '/frps' })
  if (pathname.startsWith('/frps/')) return trail('FRPS 节点详情', { title: 'FRPS 节点', path: '/frps' })

  if (pathname === '/frpc') return trail('FRPC 主机')
  if (pathname === '/frpc/new') return trail('新建 FRPC 主机', { title: 'FRPC 主机', path: '/frpc' })
  const connNew = pathname.match(/^\/frpc\/([^/]+)\/connections\/new$/)
  if (connNew) {
    return trail(
      '新增连接',
      { title: 'FRPC 主机', path: '/frpc' },
      { title: '主机详情', path: `/frpc/${connNew[1]}` },
    )
  }
  if (pathname.startsWith('/frpc/')) return trail('FRPC 主机详情', { title: 'FRPC 主机', path: '/frpc' })

  if (pathname.startsWith('/connections/')) return trail('连接详情', { title: 'FRPC 主机', path: '/frpc' })

  return { title: '总览', crumbs: [] }
}

export default function AppLayout() {
  const navigate = useNavigate()
  const location = useLocation()
  const { user, clear } = useAuth()

  const selectedKey = resolveSelectedKey(location.pathname)
  const { title: pageTitle, crumbs } = resolveNav(location.pathname)

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
          {/* Fixed-height breadcrumb line keeps the title's vertical position
              identical across pages whether or not a breadcrumb is shown. */}
          <div style={{ display: 'flex', flexDirection: 'column', justifyContent: 'center' }}>
            <div style={{ height: 18, lineHeight: '18px' }}>
              {crumbs.length > 0 && (
                <Breadcrumb
                  items={crumbs.map((c) => ({
                    title: c.path ? (
                      <a onClick={() => navigate(c.path!)} style={{ fontSize: 12 }}>{c.title}</a>
                    ) : (
                      <span style={{ fontSize: 12 }}>{c.title}</span>
                    ),
                  }))}
                />
              )}
            </div>
            <div style={{ fontSize: 16, fontWeight: 600, color: '#1a1a1a', lineHeight: '24px' }}>{pageTitle}</div>
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
