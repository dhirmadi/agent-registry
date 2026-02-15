import { useEffect, useState } from 'react';
import {
  Alert,
  Card,
  CardBody,
  CardTitle,
  Grid,
  GridItem,
  Spinner,
  Title,
} from '@patternfly/react-core';
import { api } from '../api/client';
import type { DiscoveryResponse } from '../types';

interface DashboardCounts {
  agents: number;
  mcpServers: number;
}

export function DashboardPage() {
  const [counts, setCounts] = useState<DashboardCounts | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;

    async function fetchDiscovery() {
      try {
        const data = await api.get<DiscoveryResponse>('/api/v1/discovery');
        if (!cancelled) {
          setCounts({
            agents: data.agents.length,
            mcpServers: data.mcp_servers.length,
          });
        }
      } catch (err) {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : 'Failed to load dashboard data');
        }
      } finally {
        if (!cancelled) {
          setLoading(false);
        }
      }
    }

    fetchDiscovery();

    return () => {
      cancelled = true;
    };
  }, []);

  if (loading) {
    return (
      <div data-testid="dashboard-loading" style={{ textAlign: 'center', padding: '3rem' }}>
        <Spinner aria-label="Loading dashboard" />
      </div>
    );
  }

  if (error) {
    return (
      <Alert variant="danger" title="Error loading dashboard" data-testid="dashboard-error">
        {error}
      </Alert>
    );
  }

  return (
    <div>
      <Title headingLevel="h1" style={{ marginBottom: '1.5rem' }}>
        Dashboard
      </Title>
      <Grid hasGutter>
        <GridItem span={6}>
          <Card data-testid="card-agents">
            <CardTitle>Agents</CardTitle>
            <CardBody>
              <span style={{ fontSize: '2rem', fontWeight: 700 }}>{counts?.agents ?? 0}</span>
            </CardBody>
          </Card>
        </GridItem>
        <GridItem span={6}>
          <Card data-testid="card-mcp-servers">
            <CardTitle>MCP Servers</CardTitle>
            <CardBody>
              <span style={{ fontSize: '2rem', fontWeight: 700 }}>{counts?.mcpServers ?? 0}</span>
            </CardBody>
          </Card>
        </GridItem>
      </Grid>
    </div>
  );
}
