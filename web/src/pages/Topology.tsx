import { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react'
import ReactFlow, {
  Background,
  Controls,
  Handle,
  MarkerType,
  MiniMap,
  Position,
  useEdgesState,
  useNodesState,
  type Edge,
  type Node,
  type NodeProps,
} from 'reactflow'
import 'reactflow/dist/style.css'
import { Button, Card, Drawer, Descriptions, Empty, Space, Table, Tag, Typography } from 'antd'
import { CloudServerOutlined, ApiOutlined, ReloadOutlined, EyeOutlined, EyeInvisibleOutlined } from '@ant-design/icons'
import { useNavigate } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { getTopology } from '../api/client'
import HostRuntime from '../components/HostRuntime'
import { archLabel } from '../lib/format'
import { maskIp } from '../lib/frp'
import { PROTOCOL_LABELS, protocolLabel } from '../lib/transport'
import type { FRPCConnection, FRPCHost, FRPSNode, ProxyMapping, Topology as TopologyData } from '../api/types'

const STATUS_COLOR: Record<string, string> = {
  online: '#52c41a',
  offline: '#cf1322',
  pending: '#8c8c8c',
}
const STATUS_TEXT: Record<string, string> = { online: '在线', offline: '离线', pending: '待部署' }

// HideIpContext lets the custom flow nodes (rendered by React Flow, outside the
// normal props tree) read the current hide-IP toggle without rebuilding the graph.
const HideIpContext = createContext(false)

function NodeShell({ icon, title, status, lines, accent }: { icon: React.ReactNode; title: string; status: string; lines: string[]; accent: string }) {
  return (
    <div style={{ width: 200, borderRadius: 10, border: `1px solid ${accent}`, background: '#fff', boxShadow: '0 2px 8px rgba(0,0,0,0.08)', overflow: 'hidden', fontSize: 12 }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 6, padding: '6px 10px', background: accent, color: '#fff', fontWeight: 600 }}>
        {icon}
        <span style={{ flex: 1, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{title}</span>
        <span style={{ width: 8, height: 8, borderRadius: '50%', background: STATUS_COLOR[status] ?? '#8c8c8c', boxShadow: '0 0 0 2px rgba(255,255,255,0.6)' }} />
      </div>
      <div style={{ padding: '8px 10px', lineHeight: 1.7, color: '#444' }}>
        {lines.map((l, i) => (
          <div key={i} style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{l}</div>
        ))}
      </div>
    </div>
  )
}

function FrpsFlowNode({ data }: NodeProps) {
  const n: FRPSNode = data.node
  const hideIp = useContext(HideIpContext)
  return (
    <>
      <Handle type="target" position={Position.Left} style={{ background: '#1677ff' }} />
      <NodeShell
        icon={<CloudServerOutlined />}
        title={n.name}
        status={n.status}
        accent="#1677ff"
        lines={[`公网 ${n.public_ip ? maskIp(n.public_ip, hideIp) : '未设置'}`, `端口 ${n.bind_port} · frp ${n.frp_version || '-'}`, `${archLabel(n.runtime?.os, n.runtime?.arch)}`]}
      />
    </>
  )
}

function HostFlowNode({ data }: NodeProps) {
  const h: FRPCHost = data.node
  return (
    <>
      <NodeShell
        icon={<ApiOutlined />}
        title={h.name}
        status={h.status}
        accent="#13a8a8"
        lines={[`${h.connections?.length ?? 0} 个连接`, `frp ${h.frp_version || '-'}`, archLabel(h.runtime?.os, h.runtime?.arch)]}
      />
      <Handle type="source" position={Position.Right} style={{ background: '#13a8a8' }} />
    </>
  )
}

const nodeTypes = { frps: FrpsFlowNode, host: HostFlowNode }

// buildGraph lays hosts on the left and frps on the right; each connection is one
// edge from its host to its target frps — so a host with two connections to two
// different frps shows two edges fanning out.
function buildGraph(topo: TopologyData): { nodes: Node[]; edges: Edge[] } {
  const nodes: Node[] = []
  const edges: Edge[] = []
  const ROW = 140

  topo.frps.forEach((frps, i) => {
    nodes.push({ id: `s-${frps.uuid}`, type: 'frps', position: { x: 600, y: i * ROW + 40 }, data: { node: frps } })
  })
  topo.hosts.forEach((host, i) => {
    nodes.push({ id: `h-${host.uuid}`, type: 'host', position: { x: 60, y: i * ROW + 40 }, data: { node: host } })
    for (const conn of host.connections ?? []) {
      const frps = topo.frps.find((s) => s.uuid === conn.frps_uuid)
      if (!frps) continue
      const online = conn.status === 'online'
      edges.push({
        id: conn.uuid,
        source: `h-${host.uuid}`,
        target: `s-${frps.uuid}`,
        label: `${conn.name} · ${PROTOCOL_LABELS[conn.protocol ?? 'tcp']} · ${conn.proxies?.length ?? 0}映射`,
        animated: online,
        markerEnd: { type: MarkerType.ArrowClosed },
        style: { stroke: online ? '#13a8a8' : '#bbb' },
        labelStyle: { fontSize: 11 },
        labelBgPadding: [4, 2] as [number, number],
        labelBgStyle: { fill: '#f0fdfa' },
        data: { conn, host, frps },
      })
    }
  })
  return { nodes, edges }
}

function accessAddr(p: ProxyMapping, publicIP: string | undefined, hideIp: boolean): string {
  if ((p.proxy_type === 'tcp' || p.proxy_type === 'udp') && p.remote_port) {
    return `${publicIP ? maskIp(publicIP, hideIp) : '<公网IP>'}:${p.remote_port}`
  }
  if (p.custom_domains) {
    try {
      return (JSON.parse(p.custom_domains) as string[]).join(', ')
    } catch {
      return p.custom_domains
    }
  }
  return p.subdomain || '-'
}

type Selection =
  | { kind: 'frps'; node: FRPSNode }
  | { kind: 'host'; node: FRPCHost }
  | { kind: 'edge'; conn: FRPCConnection; host: FRPCHost; frps: FRPSNode }
  | null

export default function Topology() {
  const navigate = useNavigate()
  const { data, isLoading, refetch, isFetching } = useQuery({ queryKey: ['topology'], queryFn: getTopology, refetchInterval: 15000 })

  const graph = useMemo(() => (data ? buildGraph(data) : { nodes: [], edges: [] }), [data])
  const [nodes, setNodes, onNodesChange] = useNodesState([])
  const [edges, setEdges, onEdgesChange] = useEdgesState([])
  const [sel, setSel] = useState<Selection>(null)
  const [hideIp, setHideIp] = useState(false)

  useEffect(() => {
    setNodes(graph.nodes)
    setEdges(graph.edges)
  }, [graph, setNodes, setEdges])

  const onNodeClick = useCallback((_: unknown, n: Node) => {
    if (n.type === 'frps') setSel({ kind: 'frps', node: n.data.node })
    else setSel({ kind: 'host', node: n.data.node })
  }, [])

  const onEdgeClick = useCallback((_: unknown, e: Edge) => {
    if (e.data?.conn && e.data?.frps && e.data?.host) setSel({ kind: 'edge', conn: e.data.conn, host: e.data.host, frps: e.data.frps })
  }, [])

  const empty = !isLoading && (data?.frps.length ?? 0) === 0 && (data?.hosts.length ?? 0) === 0

  return (
    <HideIpContext.Provider value={hideIp}>
      <Card
        styles={{ body: { padding: 0 } }}
        title={<Typography.Title level={4} style={{ margin: 0 }}>网络拓扑</Typography.Title>}
        extra={
          <Space>
            <Tag color="#1677ff">FRPS 服务端</Tag>
            <Tag color="#13a8a8">FRPC 主机</Tag>
            <Button
              size="small"
              icon={hideIp ? <EyeInvisibleOutlined /> : <EyeOutlined />}
              onClick={() => setHideIp((v) => !v)}
            >
              {hideIp ? '显示 IP' : '隐藏 IP'}
            </Button>
            <Button size="small" icon={<ReloadOutlined />} loading={isFetching} onClick={() => refetch()}>刷新</Button>
          </Space>
        }
      >
        <div style={{ height: 'calc(100vh - 220px)', minHeight: 480, position: 'relative' }}>
          {empty ? (
            <Empty style={{ paddingTop: 120 }} description="暂无节点，请先创建 FRPS / FRPC 主机" />
          ) : (
            <ReactFlow
              nodes={nodes}
              edges={edges}
              onNodesChange={onNodesChange}
              onEdgesChange={onEdgesChange}
              onNodeClick={onNodeClick}
              onEdgeClick={onEdgeClick}
              nodeTypes={nodeTypes}
              fitView
              minZoom={0.2}
              proOptions={{ hideAttribution: true }}
            >
              <Background gap={16} color="#eee" />
              <Controls />
              <MiniMap pannable zoomable nodeColor={(n) => (n.type === 'frps' ? '#1677ff' : '#13a8a8')} />
            </ReactFlow>
          )}
        </div>

        <DetailDrawer sel={sel} onClose={() => setSel(null)} navigate={navigate} hideIp={hideIp} />
      </Card>
    </HideIpContext.Provider>
  )
}

function DetailDrawer({ sel, onClose, navigate, hideIp }: { sel: Selection; onClose: () => void; navigate: (p: string) => void; hideIp: boolean }) {
  if (!sel) return <Drawer open={false} onClose={onClose} />

  if (sel.kind === 'edge') {
    const { conn, host, frps } = sel
    const cols = [
      { title: '名称', dataIndex: 'name' },
      { title: '协议', dataIndex: 'proxy_type', render: (v: string) => <Tag>{v.toUpperCase()}</Tag> },
      { title: '本地', render: (_: unknown, r: ProxyMapping) => `${maskIp(r.local_ip, hideIp)}:${r.local_port}` },
      { title: '访问地址', render: (_: unknown, r: ProxyMapping) => <span className="mono">{accessAddr(r, frps.public_ip, hideIp)}</span> },
    ]
    return (
      <Drawer open width={580} onClose={onClose} title={`${host.name} → ${frps.name}`} extra={<Button type="link" onClick={() => navigate(`/connections/${conn.uuid}`)}>打开连接</Button>}>
        <Descriptions column={1} bordered size="small" style={{ marginBottom: 16 }}>
          <Descriptions.Item label="连接">{conn.name}</Descriptions.Item>
          <Descriptions.Item label="传输协议"><Tag color="geekblue">{PROTOCOL_LABELS[conn.protocol ?? 'tcp']}</Tag></Descriptions.Item>
          <Descriptions.Item label="状态"><Tag color={STATUS_COLOR[conn.status]}>{STATUS_TEXT[conn.status] ?? conn.status}</Tag></Descriptions.Item>
        </Descriptions>
        <Typography.Title level={5}>端口映射</Typography.Title>
        <Table rowKey="id" size="small" pagination={false} dataSource={conn.proxies ?? []} columns={cols} />
      </Drawer>
    )
  }

  if (sel.kind === 'frps') {
    const n = sel.node
    return (
      <Drawer open width={520} onClose={onClose} title={n.name} extra={<Button type="link" onClick={() => navigate(`/frps/${n.uuid}`)}>打开详情页</Button>}>
        <Descriptions column={1} bordered size="small" style={{ marginBottom: 16 }}>
          <Descriptions.Item label="类型">FRPS 服务端</Descriptions.Item>
          <Descriptions.Item label="状态"><Tag color={STATUS_COLOR[n.status]}>{STATUS_TEXT[n.status] ?? n.status}</Tag></Descriptions.Item>
          <Descriptions.Item label="公网 IP"><span className="mono">{n.public_ip ? maskIp(n.public_ip, hideIp) : '-'}</span></Descriptions.Item>
          <Descriptions.Item label="服务端口">{n.bind_port}</Descriptions.Item>
          <Descriptions.Item label="frp 版本"><Tag>{n.frp_version || '-'}</Tag></Descriptions.Item>
        </Descriptions>
        <Typography.Title level={5}>主机运行环境</Typography.Title>
        <HostRuntime runtime={n.runtime} />
      </Drawer>
    )
  }

  // host
  const h = sel.node
  const cols = [
    { title: '连接', dataIndex: 'name' },
    { title: '传输', dataIndex: 'protocol', render: (v?: string) => <Tag color="geekblue">{protocolLabel(v)}</Tag> },
    { title: '映射', render: (_: unknown, r: FRPCConnection) => r.proxies?.length ?? 0 },
    { title: '状态', dataIndex: 'status', render: (v: string) => <Tag color={STATUS_COLOR[v]}>{STATUS_TEXT[v] ?? v}</Tag> },
  ]
  return (
    <Drawer open width={560} onClose={onClose} title={h.name} extra={<Button type="link" onClick={() => navigate(`/frpc/${h.uuid}`)}>打开主机</Button>}>
      <Descriptions column={1} bordered size="small" style={{ marginBottom: 16 }}>
        <Descriptions.Item label="类型">FRPC 主机</Descriptions.Item>
        <Descriptions.Item label="状态"><Tag color={STATUS_COLOR[h.status]}>{STATUS_TEXT[h.status] ?? h.status}</Tag></Descriptions.Item>
        <Descriptions.Item label="frp 版本"><Tag>{h.frp_version || '-'}</Tag></Descriptions.Item>
      </Descriptions>
      <Typography.Title level={5}>连接</Typography.Title>
      <Table rowKey="uuid" size="small" pagination={false} dataSource={h.connections ?? []} columns={cols} style={{ marginBottom: 16 }} />
      <Typography.Title level={5}>主机运行环境</Typography.Title>
      <HostRuntime runtime={h.runtime} />
    </Drawer>
  )
}
