import React from 'react'
import ReactDOM from 'react-dom/client'
import { BrowserRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { ConfigProvider } from 'antd'
import zhCN from 'antd/locale/zh_CN'
import dayjs from 'dayjs'
import relativeTime from 'dayjs/plugin/relativeTime'
import 'dayjs/locale/zh-cn'
import '@fontsource-variable/inter'
import App from './App'
import './index.css'

// Enable relative time ("几秒前 / 几分钟后") in Chinese for agent report timings.
dayjs.extend(relativeTime)
dayjs.locale('zh-cn')

const queryClient = new QueryClient({
  defaultOptions: { queries: { retry: 1, refetchOnWindowFocus: false } },
})

// HP-style design tokens: pure-white canvas + cloud bands, ink text, one scarce
// electric-blue accent, Inter type, sharp 4px controls against soft 16px cards,
// flat surfaces with hairline borders and a single soft-lift shadow for cards.
const FONT_FAMILY =
  "'Inter Variable', 'Inter', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'PingFang SC', 'Microsoft YaHei', sans-serif"
const SOFT_LIFT = '0 2px 8px rgba(26, 26, 26, 0.08)'
const FLOATING = '0 8px 24px rgba(26, 26, 26, 0.12)'

const theme = {
  token: {
    colorPrimary: '#024ad8', // HP Electric Blue — the lone signal color
    colorInfo: '#024ad8',
    colorLink: '#024ad8',
    colorLinkActive: '#0e3191', // primary-deep (pressed / visited)
    colorTextBase: '#1a1a1a', // ink
    colorText: '#1a1a1a',
    colorTextSecondary: '#3d3d3d', // charcoal
    colorTextTertiary: '#636363', // graphite
    colorBgLayout: '#f7f7f7', // cloud band behind floating white cards
    colorBorder: '#e8e8e8', // hairline / fog
    colorBorderSecondary: '#e8e8e8',
    colorError: '#b3262b', // bloom deep
    fontFamily: FONT_FAMILY,
    fontSize: 14,
    borderRadius: 4, // rounded.md — buttons & inputs stay sharp
    borderRadiusSM: 3,
    borderRadiusLG: 16, // rounded.xl — cards & panels stay soft
    controlHeight: 38,
    boxShadow: SOFT_LIFT,
    boxShadowSecondary: FLOATING,
    wireframe: false,
  },
  components: {
    Layout: {
      headerBg: '#ffffff',
      bodyBg: '#f7f7f7',
      siderBg: '#1a1a1a', // ink slab
      triggerBg: '#1a1a1a',
      triggerColor: '#ffffff',
    },
    Menu: {
      darkItemBg: '#1a1a1a',
      darkPopupBg: '#1a1a1a',
      darkSubMenuItemBg: '#141414',
      darkItemColor: 'rgba(255, 255, 255, 0.70)',
      darkItemHoverColor: '#ffffff',
      darkItemHoverBg: 'rgba(255, 255, 255, 0.08)',
      darkItemSelectedBg: '#024ad8', // electric-blue active indicator
      darkItemSelectedColor: '#ffffff',
      itemBorderRadius: 8,
      itemMarginInline: 8,
    },
    Card: {
      borderRadiusLG: 16,
      boxShadowTertiary: SOFT_LIFT,
      colorBorderSecondary: '#e8e8e8',
    },
    Button: {
      controlHeight: 40,
      borderRadius: 4,
      fontWeight: 600,
      primaryShadow: 'none',
      defaultShadow: 'none',
    },
    Input: { borderRadius: 4, controlHeight: 40, activeBorderColor: '#1a1a1a', hoverBorderColor: '#c2c2c2' },
    InputNumber: { borderRadius: 4, controlHeight: 40 },
    Select: { borderRadius: 4, controlHeight: 40 },
    Table: {
      headerBg: '#f7f7f7',
      headerColor: '#3d3d3d',
      borderColor: '#e8e8e8',
      rowHoverBg: '#f7f7f7',
    },
    Descriptions: { labelBg: '#f7f7f7', colorSplit: '#e8e8e8' },
    Modal: { borderRadiusLG: 16 },
    Tag: { borderRadiusSM: 8 },
  },
}

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <ConfigProvider locale={zhCN} theme={theme}>
      <QueryClientProvider client={queryClient}>
        <BrowserRouter>
          <App />
        </BrowserRouter>
      </QueryClientProvider>
    </ConfigProvider>
  </React.StrictMode>,
)
