import { useState, useEffect, useRef } from 'react';
import {
  Header,
  HeaderName,
  Content,
  Tabs,
  Tab,
  TabList,
  TabPanels,
  TabPanel,
  Tile,
  Button,
  Table,
  TableHead,
  TableRow,
  TableHeader,
  TableBody,
  TableCell,
  TableContainer,
  CodeSnippet,
  Tag,
  Search,
} from '@carbon/react';
import {
  Play,
  Stop,
  CheckmarkFilled,
  Terminal,
  ChartRing,
} from '@carbon/icons-react';

interface SystemStatus {
  engine: string;
  socket_path: string;
  active_containers: number;
  total_memory_mb: number;
  available_memory_mb: number;
}

interface ContainerService {
  container_id: string;
  name: string;
  image: string;
  status: string;
  ports: string;
}

interface ProjectInstance {
  id: string;
  name: string;
  scope: string;
  working_dir: string;
  services: ContainerService[];
}

interface RecipeModel {
  name: string;
  category: string;
  version: string;
  description: string;
  source: string;
}

interface PortAllocation {
  container_id: string;
  service_name: string;
  container_port: number;
  host_port: number;
  protocol: string;
}

function App() {
  const [activeTab, setActiveTab] = useState(0);
  const [status, setStatus] = useState<SystemStatus | null>(null);
  const [projects, setProjects] = useState<ProjectInstance[]>([]);
  const [recipes, setRecipes] = useState<RecipeModel[]>([]);
  const [ports, setPorts] = useState<PortAllocation[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Search filter for recipes
  const [recipeSearch, setRecipeSearch] = useState('');

  // Log streaming states
  const [selectedContainer, setSelectedContainer] = useState<string>('');
  const [logs, setLogs] = useState<string[]>([]);
  const eventSourceRef = useRef<EventSource | null>(null);
  const logTerminalRef = useRef<HTMLPreElement | null>(null);

  // Fetch all dashboard metrics
  const fetchData = async () => {
    try {
      setLoading(true);
      setError(null);

      // In real implementation, these call our Go HTTP server API endpoints
      const [resStatus, resProjects, resRecipes, resPorts] = await Promise.all([
        fetch('/api/status').then((r) => (r.ok ? r.json() : null)),
        fetch('/api/projects').then((r) => (r.ok ? r.json() : [])),
        fetch('/api/recipes').then((r) => (r.ok ? r.json() : [])),
        fetch('/api/ports').then((r) => (r.ok ? r.json() : [])),
      ]);

      if (resStatus) setStatus(resStatus);
      setProjects(resProjects || []);
      setRecipes(resRecipes || []);
      setPorts(resPorts || []);
    } catch (err: any) {
      setError('Failed to connect to LBMesh API backend. Ensure the CLI server is running.');
      // Fallback mock data for styling and standalone previews
      setMockData();
    } finally {
      setLoading(false);
    }
  };

  const setMockData = () => {
    setStatus({
      engine: 'Podman (Rootless)',
      socket_path: '/Users/developer/.local/share/containers/podman/machine/qemu/podman.sock',
      active_containers: 3,
      total_memory_mb: 4096,
      available_memory_mb: 2840,
    });

    setProjects([
      {
        id: "system-services",
        name: "Global shared stacks",
        scope: "system",
        working_dir: "~/.lbmesh/system",
        services: [
          { container_id: "71e54911d", name: "lbmesh-global-proxy", image: "nginx:alpine", status: "running", ports: "80:80, 443:443" }
        ]
      },
      {
        id: "project-ecommerce",
        name: "ecommerce-platform",
        scope: "project",
        working_dir: "/Users/developer/Workspace/ecommerce",
        services: [
          { container_id: "a3f4e223d", name: "postgres-db", image: "postgres:16-alpine", status: "running", ports: "5432:5432" },
          { container_id: "bc847112a", name: "node-backend", image: "node:20-alpine", status: "running", ports: "8080:8080" },
          { container_id: "f3e1a8e88", name: "frontend-spa", image: "nginx:alpine", status: "exited", ports: "3000:80" }
        ]
      }
    ]);

    setRecipes([
      { name: "postgres", category: "database", version: "16-alpine", description: "Relational database optimized for local developer projects.", source: "embedded" },
      { name: "redis", category: "database", version: "7-alpine", description: "In-memory caching store with pre-negotiated port maps.", source: "embedded" },
      { name: "nginx", category: "proxy", version: "alpine", description: "Static file host and local microservice router bridge.", source: "embedded" },
      { name: "python-llm", category: "ai-agent", version: "3.11", description: "LiteLLM local deployment mapped with dynamic CUDA memory checks.", source: "custom" },
      { name: "corporate-auth", category: "backend", version: "v1", description: "Security tap container mimicking production LDAP gates.", source: "tap" }
    ]);

    setPorts([
      { container_id: "71e54911d", service_name: "lbmesh-global-proxy", container_port: 80, host_port: 80, protocol: "tcp" },
      { container_id: "a3f4e223d", service_name: "postgres-db", container_port: 5432, host_port: 5432, protocol: "tcp" },
      { container_id: "bc847112a", service_name: "node-backend", container_port: 8080, host_port: 8080, protocol: "tcp" }
    ]);
  };

  useEffect(() => {
    fetchData();
    const interval = setInterval(fetchData, 10000); // Poll status every 10s
    return () => clearInterval(interval);
  }, []);

  // Handle EventSource for Log Streaming
  useEffect(() => {
    if (!selectedContainer) {
      setLogs([]);
      return;
    }

    setLogs([`Connecting to ${selectedContainer} log stream...`]);

    if (eventSourceRef.current) {
      eventSourceRef.current.close();
    }

    // Connect to native Go Server-Sent Events endpoint
    const es = new EventSource(`/api/logs?container=${selectedContainer}`);
    eventSourceRef.current = es;

    es.onmessage = (event) => {
      setLogs((prev) => [...prev, event.data]);
    };

    es.onerror = () => {
      setLogs((prev) => [...prev, "Connection to log stream suspended or container stopped."]);
      es.close();
    };

    return () => {
      if (es) es.close();
    };
  }, [selectedContainer]);

  // Auto-scroll logs to bottom
  useEffect(() => {
    if (logTerminalRef.current) {
      logTerminalRef.current.scrollTop = logTerminalRef.current.scrollHeight;
    }
  }, [logs]);

  // Handle up/down action triggers
  const handleProjectAction = async (projectId: string, action: 'up' | 'down') => {
    try {
      setLoading(true);
      const res = await fetch(`/api/projects/${projectId}/${action}`, {
        method: 'POST',
      });
      if (!res.ok) {
        const errTxt = await res.text();
        alert(`Failed to perform action ${action}: ${errTxt}`);
      }
      fetchData();
    } catch (err) {
      alert(`API Connection error on project action.`);
    } finally {
      setLoading(false);
    }
  };

  const activeContainers = projects.flatMap((p) => p.services).filter((s) => s.status === 'running');

  return (
    <div style={{ minHeight: '100vh', display: 'flex', flexDirection: 'column' }}>
      <Header aria-label="LBMesh Dashboard">
        <HeaderName href="#" prefix="LBMesh">
          Container Platform
        </HeaderName>
      </Header>

      <Content style={{ marginTop: '3rem', flex: 1, backgroundColor: '#f4f4f4', padding: '2rem' }}>
        {error && (
          <div style={{ display: 'flex', gap: '1rem', alignItems: 'center', backgroundColor: '#fff', padding: '1rem', borderLeft: '4px solid #f1c21b', marginBottom: '1.5rem', borderRadius: '4px' }}>
            <div style={{ flex: 1, textAlign: 'left' }}>
              <strong style={{ display: 'block', color: '#161616', fontSize: '0.975rem' }}>Standalone Simulator Mode</strong>
              <span style={{ fontSize: '0.875rem', color: '#525252' }}>{error}</span>
            </div>
            <Button onClick={fetchData} size="sm" kind="tertiary">Retry Connection</Button>
          </div>
        )}

        <Tabs selectedIndex={activeTab} onChange={(index) => setActiveTab(index.selectedIndex)}>
          <TabList aria-label="LBMesh Sections">
            <Tab>System Overview</Tab>
            <Tab>Projects & Services</Tab>
            <Tab>Recipe Catalog</Tab>
            <Tab>Port Allocations</Tab>
            <Tab>Live Container Logs</Tab>
          </TabList>

          <TabPanels>
            {/* TAB 1: SYSTEM OVERVIEW */}
            <TabPanel>
              <div className="cds--grid" style={{ padding: '1.5rem 0 0 0' }}>
                <div className="cds--row" style={{ marginBottom: '1.5rem' }}>
                  <div className="cds--col-md-4 cds--col-lg-4" style={{ marginBottom: '1rem' }}>
                    <Tile style={{ minHeight: '150px', backgroundColor: 'white' }}>
                      <h4 style={{ color: '#525252', fontSize: '0.875rem', fontWeight: 500, marginBottom: '0.5rem' }}>DAEMON ENGINE</h4>
                      <h2 style={{ fontSize: '1.75rem', fontWeight: 600, color: '#161616', marginBottom: '1rem' }}>
                        {status?.engine || 'Detecting...'}
                      </h2>
                      <div style={{ display: 'flex', alignItems: 'center' }}>
                        <CheckmarkFilled style={{ fill: '#24a148', marginRight: '0.5rem' }} />
                        <span style={{ fontSize: '0.875rem', color: '#525252' }}>Socket connected</span>
                      </div>
                    </Tile>
                  </div>

                  <div className="cds--col-md-4 cds--col-lg-4" style={{ marginBottom: '1rem' }}>
                    <Tile style={{ minHeight: '150px', backgroundColor: 'white' }}>
                      <h4 style={{ color: '#525252', fontSize: '0.875rem', fontWeight: 500, marginBottom: '0.5rem' }}>ACTIVE SERVICES</h4>
                      <h2 style={{ fontSize: '2.5rem', fontWeight: 600, color: '#161616', marginBottom: '0.5rem' }}>
                        {status?.active_containers ?? 0}
                      </h2>
                      <span style={{ fontSize: '0.875rem', color: '#525252' }}>Containers running across all scopes</span>
                    </Tile>
                  </div>

                  <div className="cds--col-md-4 cds--col-lg-4" style={{ marginBottom: '1rem' }}>
                    <Tile style={{ minHeight: '150px', backgroundColor: 'white' }}>
                      <h4 style={{ color: '#525252', fontSize: '0.875rem', fontWeight: 500, marginBottom: '0.5rem' }}>HYPERVISOR MEMORY USAGE</h4>
                      <h2 style={{ fontSize: '1.75rem', fontWeight: 600, color: '#161616', marginBottom: '0.5rem' }}>
                        {status ? `${status.total_memory_mb - status.available_memory_mb}MB / ${status.total_memory_mb}MB` : '0MB / 0MB'}
                      </h2>
                      <div style={{ display: 'flex', alignItems: 'center', marginTop: '0.5rem' }}>
                        <ChartRing style={{ fill: '#0f62fe', marginRight: '0.5rem' }} />
                        <span style={{ fontSize: '0.875rem', color: '#525252' }}>
                          {status ? `${Math.round(((status.total_memory_mb - status.available_memory_mb) / status.total_memory_mb) * 100)}% allocated` : 'OOM Guard active'}
                        </span>
                      </div>
                    </Tile>
                  </div>
                </div>

                <div className="cds--row">
                  <div className="cds--col-lg-12">
                    <Tile style={{ backgroundColor: 'white' }}>
                      <h3 style={{ fontSize: '1.25rem', fontWeight: 600, color: '#161616', marginBottom: '1rem' }}>Direct Socket Endpoint</h3>
                      <p style={{ fontSize: '0.875rem', color: '#525252', marginBottom: '1rem' }}>
                        LBMesh bypasses the external terminal commands wrapper entirely. It connects and streams JSON configurations natively over UNIX HTTP socket streams.
                      </p>
                      <CodeSnippet type="single" feedback="Copied socket path">
                        {status?.socket_path || 'unix:///var/run/docker.sock'}
                      </CodeSnippet>
                    </Tile>
                  </div>
                </div>
              </div>
            </TabPanel>

            {/* TAB 2: PROJECTS & SERVICES */}
            <TabPanel>
              <div style={{ padding: '1.5rem 0' }}>
                {projects.map((proj) => (
                  <div key={proj.id} style={{ marginBottom: '2rem' }}>
                    <Tile style={{ backgroundColor: 'white', padding: '1.5rem' }}>
                      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '1rem', flexWrap: 'wrap', gap: '1rem' }}>
                        <div>
                          <div style={{ display: 'flex', alignItems: 'center', gap: '0.5rem' }}>
                            <h3 style={{ fontSize: '1.25rem', fontWeight: 600, color: '#161616' }}>{proj.name}</h3>
                            <Tag type={proj.scope === 'system' ? 'purple' : proj.scope === 'custom' ? 'teal' : 'cyan'}>
                              {proj.scope.toUpperCase()} SCOPE
                            </Tag>
                          </div>
                          {proj.working_dir && (
                            <span style={{ fontSize: '0.75rem', color: '#6f6f6f', fontFamily: 'monospace' }}>
                              Dir: {proj.working_dir}
                            </span>
                          )}
                        </div>

                        <div style={{ display: 'flex', gap: '0.5rem' }}>
                          {proj.scope !== 'system' && (
                            <>
                              <Button
                                renderIcon={Play}
                                size="sm"
                                kind="ghost"
                                onClick={() => handleProjectAction(proj.id, 'up')}
                                disabled={loading}
                              >
                                up
                              </Button>
                              <Button
                                renderIcon={Stop}
                                size="sm"
                                kind="danger--ghost"
                                onClick={() => handleProjectAction(proj.id, 'down')}
                                disabled={loading}
                              >
                                down
                              </Button>
                            </>
                          )}
                        </div>
                      </div>

                      <TableContainer>
                        <Table size="lg">
                          <TableHead>
                            <TableRow>
                              <TableHeader>CONTAINER ID</TableHeader>
                              <TableHeader>SERVICE NAME</TableHeader>
                              <TableHeader>DOCKER IMAGE</TableHeader>
                              <TableHeader>ROUTING PORTS</TableHeader>
                              <TableHeader>STATUS</TableHeader>
                              <TableRow></TableRow>
                            </TableRow>
                          </TableHead>
                          <TableBody>
                            {proj.services.length === 0 ? (
                              <TableRow>
                                <TableCell colSpan={6} style={{ textAlign: 'center', color: '#6f6f6f', padding: '2rem' }}>
                                  No active containers in this scope. Run "up" to provision resources.
                                </TableCell>
                              </TableRow>
                            ) : (
                              proj.services.map((srv) => (
                                <TableRow key={srv.container_id}>
                                  <TableCell style={{ fontFamily: 'monospace' }}>{srv.container_id.substring(0, 12)}</TableCell>
                                  <TableCell style={{ fontWeight: 600, color: '#161616' }}>{srv.name}</TableCell>
                                  <TableCell>{srv.image}</TableCell>
                                  <TableCell style={{ fontFamily: 'monospace' }}>{srv.ports || 'internal'}</TableCell>
                                  <TableCell>
                                    <Tag type={srv.status.toLowerCase() === 'running' ? 'green' : 'red'}>
                                      {srv.status.toUpperCase()}
                                    </Tag>
                                  </TableCell>
                                  <TableCell>
                                    {srv.status.toLowerCase() === 'running' && (
                                      <Button
                                        size="xs"
                                        kind="ghost"
                                        renderIcon={Terminal}
                                        hasIconOnly
                                        iconDescription="View Logs"
                                        onClick={() => {
                                          setSelectedContainer(srv.container_id);
                                          setActiveTab(4); // Switch to Logs tab
                                        }}
                                      />
                                    )}
                                  </TableCell>
                                </TableRow>
                              ))
                            )}
                          </TableBody>
                        </Table>
                      </TableContainer>
                    </Tile>
                  </div>
                ))}
              </div>
            </TabPanel>

            {/* TAB 3: RECIPE CATALOG */}
            <TabPanel>
              <div style={{ padding: '1.5rem 0' }}>
                <TableContainer title="Cascading Recipe Registry" description="All templates are consolidated from local directory checkpaths, private Git Taps, and in-binary fallback structures.">
                  <div style={{ marginBottom: '1rem', width: '300px' }}>
                    <Search
                      size="lg"
                      labelText="Search recipes"
                      closeButtonLabelText="Clear search input"
                      placeholder="Search name, category or tag..."
                      id="recipe-search"
                      value={recipeSearch}
                      onChange={(e: any) => setRecipeSearch(e.target.value)}
                    />
                  </div>
                  <Table size="lg">
                    <TableHead>
                      <TableRow>
                        <TableHeader>RECIPE NAME</TableHeader>
                        <TableHeader>CATEGORY</TableHeader>
                        <TableHeader>VERSION</TableHeader>
                        <TableHeader>DESCRIPTION</TableHeader>
                        <TableHeader>CATALOG SOURCE</TableHeader>
                      </TableRow>
                    </TableHead>
                    <TableBody>
                      {recipes
                        .filter((r) =>
                          r.name.toLowerCase().includes(recipeSearch.toLowerCase()) ||
                          r.category.toLowerCase().includes(recipeSearch.toLowerCase()) ||
                          r.source.toLowerCase().includes(recipeSearch.toLowerCase())
                        )
                        .map((rec) => (
                          <TableRow key={rec.name}>
                            <TableCell style={{ fontWeight: 600, color: '#0f62fe' }}>{rec.name}</TableCell>
                            <TableCell style={{ textTransform: 'capitalize' }}>{rec.category}</TableCell>
                            <TableCell style={{ fontFamily: 'monospace' }}>{rec.version}</TableCell>
                            <TableCell style={{ maxWidth: '400px' }}>{rec.description}</TableCell>
                            <TableCell>
                              <Tag type={rec.source === 'embedded' ? 'cyan' : rec.source === 'tap' ? 'purple' : 'green'}>
                                {rec.source.toUpperCase()}
                              </Tag>
                            </TableCell>
                          </TableRow>
                        ))}
                    </TableBody>
                  </Table>
                </TableContainer>
              </div>
            </TabPanel>

            {/* TAB 4: PORT ALLOCATIONS */}
            <TabPanel>
              <div style={{ padding: '1.5rem 0' }}>
                <TableContainer title="Dynamic TCP Port Mappings" description="Active reservations tracked in state.db. All conflicting requests are re-mapped dynamically on collision.">
                  <Table size="lg">
                    <TableHead>
                      <TableRow>
                        <TableHeader>SERVICE NAME</TableHeader>
                        <TableHeader>CONTAINER PORT</TableHeader>
                        <TableHeader>ALLOCATED HOST PORT</TableHeader>
                        <TableHeader>PROTOCOL</TableHeader>
                        <TableHeader>ROUTING TARGET</TableHeader>
                      </TableRow>
                    </TableHead>
                    <TableBody>
                      {ports.length === 0 ? (
                        <TableRow>
                          <TableCell colSpan={5} style={{ textAlign: 'center', color: '#6f6f6f', padding: '2rem' }}>
                            No port reservations are registered in the state engine.
                          </TableCell>
                        </TableRow>
                      ) : (
                        ports.map((p, idx) => (
                          <TableRow key={idx}>
                            <TableCell style={{ fontWeight: 600, color: '#161616' }}>{p.service_name}</TableCell>
                            <TableCell style={{ fontFamily: 'monospace' }}>{p.container_port}</TableCell>
                            <TableCell style={{ fontFamily: 'monospace', fontWeight: 600, color: '#0f62fe' }}>
                              {p.host_port}
                            </TableCell>
                            <TableCell style={{ textTransform: 'uppercase' }}>{p.protocol}</TableCell>
                            <TableCell style={{ fontFamily: 'monospace' }}>
                              127.0.0.1:{p.host_port} &rarr; container:{p.container_port}
                            </TableCell>
                          </TableRow>
                        ))
                      )}
                    </TableBody>
                  </Table>
                </TableContainer>
              </div>
            </TabPanel>

            {/* TAB 5: LIVE CONTAINER LOGS */}
            <TabPanel>
              <div style={{ padding: '1.5rem 0' }}>
                <Tile style={{ backgroundColor: 'white', padding: '1.5rem' }}>
                  <div style={{ marginBottom: '1.5rem', display: 'flex', alignItems: 'center', gap: '1rem', flexWrap: 'wrap' }}>
                    <div style={{ minWidth: '250px' }}>
                      <label htmlFor="container-select" style={{ display: 'block', fontSize: '0.875rem', fontWeight: 500, color: '#525252', marginBottom: '0.5rem' }}>
                        Select Active Container Stream:
                      </label>
                      <select
                        id="container-select"
                        value={selectedContainer}
                        onChange={(e) => setSelectedContainer(e.target.value)}
                        style={{
                          width: '100%',
                          padding: '0.5rem',
                          borderRadius: '4px',
                          border: '1px solid #8d8d8d',
                          backgroundColor: 'white',
                          fontFamily: 'monospace',
                        }}
                      >
                        <option value="">-- Choose Container --</option>
                        {activeContainers.map((c) => (
                          <option key={c.container_id} value={c.container_id}>
                            {c.name} ({c.container_id.substring(0, 12)})
                          </option>
                        ))}
                      </select>
                    </div>

                    {selectedContainer && (
                      <Button
                        size="sm"
                        kind="ghost"
                        onClick={() => {
                          const container = selectedContainer;
                          setSelectedContainer('');
                          setTimeout(() => setSelectedContainer(container), 100);
                        }}
                      >
                        Clear & Restart Stream
                      </Button>
                    )}
                  </div>

                  <pre
                    ref={logTerminalRef}
                    className="terminal-logs"
                    style={{
                      backgroundColor: '#161616',
                      color: '#f4f4f4',
                      padding: '1.5rem',
                      fontFamily: 'monospace',
                      maxHeight: '500px',
                      overflowY: 'auto',
                      borderRadius: '4px',
                      whiteSpace: 'pre-wrap',
                      fontSize: '0.875rem',
                      lineHeight: '1.4',
                    }}
                  >
                    {logs.length === 0
                      ? 'Select an active container from the dropdown list above to begin streaming logs via Server-Sent Events...'
                      : logs.join('\n')}
                  </pre>
                </Tile>
              </div>
            </TabPanel>
          </TabPanels>
        </Tabs>
      </Content>

      <footer
        style={{
          padding: '1.5rem',
          textAlign: 'center',
          fontSize: '0.75rem',
          color: '#8d8d8d',
          backgroundColor: '#e0e0e0',
          borderTop: '1px solid #dcdcdc',
        }}
      >
        LBMesh Local Core Engine — Made with IBM Carbon Design System & Go Sockets.
      </footer>
    </div>
  );
}

export default App;
