import { useCallback, useEffect, useMemo, useState } from 'react'
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
import { CloudServerOutlined, ApiOutlined, ReloadOutlined } from '@ant-design/icons'
import { useNavigate } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { getTopology } from '../api/client'
import HostRuntime from '../components/HostRuntime'
import { archLabel } from '../lib/format'
import type { FRPCClient, FRPSNode, ProxyMapping, Topology as TopologyData } from '../api/types'

const STATUS_COLOR: Record<string, string> = {
  online: '#52c41a',
  offline: '#cf1322',
  pending: '#8c8c8c',
}
const STATUS_TEXT: Record<string, string> = { online: '在线', offline: '离线', pending: '待部署' }

// ---- custom nodes ----

function NodeShell({
  icon,
  title,
  status,
  lines,
  accent,
}: {
  icon: React.ReactNode
  title: string
  status: string
  lines: string[]
  accent: string
}) {
  return (
    <div
      style={{
        width: 200,
        borderRadius: 10,
        border: `1px solid ${accent}`,
        background: '#fff',
        boxShadow: '0 2px 8px rgba(0,0,0,0.08)',
        overflow: 'hidden',
        fontSize: 12,
      }}
    >
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
  return (
    <>
      <Handle type="target" position={Position.Left} style={{ background: '#1677ff' }} />
      <NodeShell
        icon={<CloudServerOutlined />}
        title={n.name}
        status={n.status}
        accent="#1677ff"
        lines={[
          `公网 ${n.public_ip || '未设置'}`,
          `端口 ${n.bind_port} · frp ${n.frp_version || '-'}`,
          `${archLabel(n.runtime?.os, n.runtime?.arch)} · 连接 ${n.runtime?.active_connections ?? 0}`,
        ]}
      />
    </>
  )
}

function FrpcFlowNode({ data }: NodeProps) {
  const c: FRPCClient = data.node
  return (
    <>
      <NodeShell
        icon={<ApiOutlined />}
        title={c.name}
        status={c.status}
        accent="#13a8a8"
        lines={[
          `${c.proxies?.length ?? 0} 个端口映射`,
          `frp ${c.frp_version || '-'}`,
          archLabel(c.runtime?.os, c.runtime?.arch),
        ]}
      />
      <Handle type="source" position={Position.Right} style={{ background: '#13a8a8' }} />
    </>
  )
}

const nodeTypes = { frps: FrpsFlowNode, frpc: FrpcFlowNode }

// ---- layout ----

function buildGraph(topo: TopologyData): { nodes: Node[]; edges: Edge[] } {
  const nodes: Node[] = []
  const edges: Edge[] = []
  const ROW = 150
  const GAP = 50
  let cursor = 0

  for (const frps of topo.frps) {
    const clients = topo.frpc.filter((c) => c.frps_uuid === frps.uuid)
    const groupStart = cursor
    const span = Math.max(clients.length, 1)

    clients.forEach((c) => {
      nodes.push({ id: `c-${c.uuid}`, type: 'frpc', position: { x: 60, y: cursor * ROW + 40 }, data: { node: c } })
      edges.push({
        id: `e-${c.uuid}`,
        source: `c-${c.uuid}`,
        target: `s-${frps.uuid}`,
        label: `${c.proxies?.length ?? 0} 映射`,
        animated: c.status === 'online' && frps.status === 'online',
        markerEnd: { type: MarkerType.ArrowClosed },
        style: { stroke: c.status === 'online' ? '#13a8a8' : '#bbb' },
        labelStyle: { fontSize: 11 },
        labelBgPadding: [4, 2] as [number, number],
        labelBgStyle: { fill: '#f0fdfa' },
        data: { frpc: c, frps },
      })
      cursor += 1
    })

    const centerY = (groupStart + span / 2) * ROW + 40 - ROW / 2
    nodes.push({ id: `s-${frps.uuid}`, type: 'frps', position: { x: 520, y: centerY }, data: { node: frps } })
    cursor = groupStart + span
    cursor += GAP / ROW // small gap between groups (fractional rows)
    cursor = Math.ceil(cursor)
  }

  // Orphan frpc clients whose target frps no longer exists.
  const orphans = topo.frpc.filter((c) => !topo.frps.some((s) => s.uuid === c.frps_uuid))
  orphans.forEach((c) => {
    nodes.push({ id: `c-${c.uuid}`, type: 'frpc', position: { x: 60, y: cursor * ROW + 40 }, data: { node: c } })
    cursor += 1
  })

  return { nodes, edges }
}

// ---- access address helper ----

function accessAddr(p: ProxyMapping, publicIP?: string): string {
  if ((p.proxy_type === 'tcp' || p.proxy_type === 'udp') && p.remote_port) {
    return `${publicIP || '<公网IP>'}:${p.remote_port}`
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
  | { kind: 'frpc'; node: FRPCClient; frps?: FRPSNode }
  | { kind: 'edge'; frpc: FRPCClient; frps: FRPSNode }
  | null

export default function Topology() {
  const navigate = useNavigate()
  const { data, isLoading, refetch, isFetching } = useQuery({
    queryKey: ['topology'],
    queryFn: getTopology,
    refetchInterval: 15000,
  })

  const graph = useMemo(() => (data ? buildGraph(data) : { nodes: [], edges: [] }), [data])
  const [nodes, setNodes, onNodesChange] = useNodesState([])
  const [edges, setEdges, onEdgesChange] = useEdgesState([])
  const [sel, setSel] = useState<Selection>(null)

  useEffect(() => {
    setNodes(graph.nodes)
    setEdges(graph.edges)
  }, [graph, setNodes, setEdges])

  const onNodeClick = useCallback(
    (_: unknown, n: Node) => {
      if (n.type === 'frps') {
        setSel({ kind: 'frps', node: n.data.node })
      } else {
        const c: FRPCClient = n.data.node
        const frps = data?.frps.find((s) => s.uuid === c.frps_uuid)
        setSel({ kind: 'frpc', node: c, frps })
      }
    },
    [data],
  )

  const onEdgeClick = useCallback((_: unknown, e: Edge) => {
    if (e.data?.frpc && e.data?.frps) setSel({ kind: 'edge', frpc: e.data.frpc, frps: e.data.frps })
  }, [])

  const empty = !isLoading && (data?.frps.length ?? 0) === 0 && (data?.frpc.length ?? 0) === 0

  return (
    <Card
      styles={{ body: { padding: 0 } }}
      title={<Typography.Title level={4} style={{ margin: 0 }}>网络拓扑</Typography.Title>}
      extra={
        <Space>
          <Tag color="#1677ff">FRPS 服务端</Tag>
          <Tag color="#13a8a8">FRPC 客户端</Tag>
          <Button size="small" icon={<ReloadOutlined />} loading={isFetching} onClick={() => refetch()}>刷新</Button>
        </Space>
      }
    >
      <div style={{ height: 'calc(100vh - 220px)', minHeight: 480, position: 'relative' }}>
        {empty ? (
          <Empty style={{ paddingTop: 120 }} description="暂无节点，请先创建 FRPS / FRPC" />
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

      <DetailDrawer sel={sel} onClose={() => setSel(null)} navigate={navigate} />
    </Card>
  )
}

function DetailDrawer({ sel, onClose, navigate }: { sel: Selection; onClose: () => void; navigate: (p: string) => void }) {
  if (!sel) return <Drawer open={false} onClose={onClose} />

  if (sel.kind === 'edge') {
    const { frpc, frps } = sel
    const cols = [
      { title: '名称', dataIndex: 'name' },
      { title: '协议', dataIndex: 'proxy_type', render: (v: string) => <Tag>{v.toUpperCase()}</Tag> },
      { title: '本地', render: (_: unknown, r: ProxyMapping) => `${r.local_ip}:${r.local_port}` },
      { title: '访问地址', render: (_: unknown, r: ProxyMapping) => <span className="mono">{accessAddr(r, frps.public_ip)}</span> },
    ]
    return (
      <Drawer open width={560} onClose={onClose} title={`${frpc.name} → ${frps.name}`}>
        <Typography.Paragraph type="secondary">该客户端通过此 FRPS 暴露的端口映射：</Typography.Paragraph>
        <Table rowKey="id" size="small" pagination={false} dataSource={frpc.proxies ?? []} columns={cols} />
      </Drawer>
    )
  }

  if (sel.kind === 'frps') {
    const n = sel.node
    return (
      <Drawer
        open
        width={520}
        onClose={onClose}
        title={n.name}
        extra={<Button type="link" onClick={() => navigate(`/frps/${n.uuid}`)}>打开详情页</Button>}
      >
        <Descriptions column={1} bordered size="small" style={{ marginBottom: 16 }}>
          <Descriptions.Item label="类型">FRPS 服务端</Descriptions.Item>
          <Descriptions.Item label="状态"><Tag color={STATUS_COLOR[n.status]}>{STATUS_TEXT[n.status] ?? n.status}</Tag></Descriptions.Item>
          <Descriptions.Item label="公网 IP"><span className="mono">{n.public_ip || '-'}</span></Descriptions.Item>
          <Descriptions.Item label="服务端口">{n.bind_port}</Descriptions.Item>
          <Descriptions.Item label="Dashboard 端口">{n.dashboard_port || '未启用'}</Descriptions.Item>
          <Descriptions.Item label="frp 版本"><Tag>{n.frp_version || '-'}</Tag></Descriptions.Item>
        </Descriptions>
        <Typography.Title level={5}>主机运行环境</Typography.Title>
        <HostRuntime runtime={n.runtime} />
      </Drawer>
    )
  }

  // frpc
  const c = sel.node
  const cols = [
    { title: '名称', dataIndex: 'name' },
    { title: '协议', dataIndex: 'proxy_type', render: (v: string) => <Tag>{v.toUpperCase()}</Tag> },
    { title: '本地', render: (_: unknown, r: ProxyMapping) => `${r.local_ip}:${r.local_port}` },
    { title: '访问地址', render: (_: unknown, r: ProxyMapping) => <span className="mono">{accessAddr(r, sel.frps?.public_ip)}</span> },
  ]
  return (
    <Drawer
      open
      width={560}
      onClose={onClose}
      title={c.name}
      extra={<Button type="link" onClick={() => navigate(`/frpc/${c.uuid}`)}>打开详情页</Button>}
    >
      <Descriptions column={1} bordered size="small" style={{ marginBottom: 16 }}>
        <Descriptions.Item label="类型">FRPC 客户端</Descriptions.Item>
        <Descriptions.Item label="状态"><Tag color={STATUS_COLOR[c.status]}>{STATUS_TEXT[c.status] ?? c.status}</Tag></Descriptions.Item>
        <Descriptions.Item label="目标 FRPS">{sel.frps?.name ?? c.frps_uuid}</Descriptions.Item>
        <Descriptions.Item label="frp 版本"><Tag>{c.frp_version || '-'}</Tag></Descriptions.Item>
      </Descriptions>
      <Typography.Title level={5}>端口映射</Typography.Title>
      <Table rowKey="id" size="small" pagination={false} dataSource={c.proxies ?? []} columns={cols} style={{ marginBottom: 16 }} />
      <Typography.Title level={5}>主机运行环境</Typography.Title>
      <HostRuntime runtime={c.runtime} />
    </Drawer>
  )
}
