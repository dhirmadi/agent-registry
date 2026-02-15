import { Fragment, useEffect, useState, useCallback, useRef } from 'react';
import {
  Alert,
  Button,
  CodeBlock,
  CodeBlockCode,
  Label,
  Spinner,
  TextInput,
  Title,
  Toolbar,
  ToolbarContent,
  ToolbarItem,
} from '@patternfly/react-core';
import { Table, Thead, Tr, Th, Tbody, Td } from '@patternfly/react-table';
import { api } from '../api/client';
import type { A2AAgentCard, A2AIndexResponse } from '../types';

export function A2APage() {
  // Well-known card state
  const [wellKnownCard, setWellKnownCard] = useState<A2AAgentCard | null>(null);
  const [wellKnownEtag, setWellKnownEtag] = useState<string>('');
  const [wellKnownLoading, setWellKnownLoading] = useState(true);
  const [wellKnownError, setWellKnownError] = useState<string | null>(null);

  // Index state
  const [cards, setCards] = useState<A2AAgentCard[]>([]);
  const [total, setTotal] = useState(0);
  const [indexLoading, setIndexLoading] = useState(true);
  const [indexError, setIndexError] = useState<string | null>(null);
  const [offset, setOffset] = useState(0);
  const [search, setSearch] = useState('');
  const [expandedRows, setExpandedRows] = useState<Set<number>>(new Set());
  const limit = 20;

  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // Fetch well-known card (raw JSON, no envelope)
  const fetchWellKnown = useCallback(async () => {
    setWellKnownLoading(true);
    setWellKnownError(null);
    try {
      const response = await fetch('/.well-known/agent.json', {
        credentials: 'same-origin',
      });
      if (!response.ok) {
        throw new Error(`HTTP ${response.status}`);
      }
      const etag = response.headers.get('ETag') || '';
      const data = await response.json();
      setWellKnownCard(data);
      setWellKnownEtag(etag);
    } catch (err) {
      setWellKnownError(err instanceof Error ? err.message : 'Failed to load well-known card');
    } finally {
      setWellKnownLoading(false);
    }
  }, []);

  // Fetch index (standard API envelope)
  const fetchIndex = useCallback(async (q: string, pageOffset: number) => {
    setIndexLoading(true);
    setIndexError(null);
    try {
      let path = `/api/v1/agents/a2a-index?offset=${pageOffset}&limit=${limit}`;
      if (q) {
        path += `&q=${encodeURIComponent(q)}`;
      }
      const data = await api.get<A2AIndexResponse>(path);
      setCards(data.agent_cards ?? []);
      setTotal(data.total ?? 0);
    } catch (err) {
      setIndexError(err instanceof Error ? err.message : 'Failed to load agent cards');
    } finally {
      setIndexLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchWellKnown();
    fetchIndex('', 0);
  }, [fetchWellKnown, fetchIndex]);

  function handleSearchChange(_event: React.FormEvent<HTMLInputElement>, value: string) {
    setSearch(value);
    if (debounceRef.current) {
      clearTimeout(debounceRef.current);
    }
    debounceRef.current = setTimeout(() => {
      setOffset(0);
      fetchIndex(value, 0);
    }, 300);
  }

  function handleNextPage() {
    const newOffset = offset + limit;
    setOffset(newOffset);
    fetchIndex(search, newOffset);
  }

  function handlePrevPage() {
    const newOffset = Math.max(0, offset - limit);
    setOffset(newOffset);
    fetchIndex(search, newOffset);
  }

  function toggleRow(index: number) {
    setExpandedRows((prev) => {
      const next = new Set(prev);
      if (next.has(index)) {
        next.delete(index);
      } else {
        next.add(index);
      }
      return next;
    });
  }

  async function copyToClipboard(text: string) {
    try {
      await navigator.clipboard.writeText(text);
    } catch {
      // Fallback: noop in test environments
    }
  }

  const currentPage = Math.floor(offset / limit) + 1;
  const totalPages = Math.ceil(total / limit) || 1;

  return (
    <div>
      <Title headingLevel="h1" style={{ marginBottom: '1.5rem' }}>
        A2A Agent Cards
      </Title>

      {/* Section 1: Well-Known Card Preview */}
      <Title headingLevel="h2" size="lg" style={{ marginBottom: '1rem' }}>
        Well-Known Card Preview
      </Title>

      {wellKnownLoading ? (
        <div data-testid="wellknown-loading" style={{ textAlign: 'center', padding: '2rem' }}>
          <Spinner aria-label="Loading well-known card" />
        </div>
      ) : wellKnownError ? (
        <Alert variant="danger" title="Error loading well-known card" data-testid="wellknown-error" isInline>
          {wellKnownError}
        </Alert>
      ) : wellKnownCard ? (
        <div data-testid="wellknown-card" style={{ marginBottom: '2rem' }}>
          <Toolbar>
            <ToolbarContent>
              <ToolbarItem>
                <Label color="blue">ETag: {wellKnownEtag}</Label>
              </ToolbarItem>
              <ToolbarItem>
                <Button
                  variant="secondary"
                  size="sm"
                  data-testid="copy-wellknown-url"
                  onClick={() => copyToClipboard(`${window.location.origin}/.well-known/agent.json`)}
                >
                  Copy URL
                </Button>
              </ToolbarItem>
            </ToolbarContent>
          </Toolbar>
          <CodeBlock>
            <CodeBlockCode>{JSON.stringify(wellKnownCard, null, 2)}</CodeBlockCode>
          </CodeBlock>
        </div>
      ) : null}

      {/* Section 2: Agent Card Index */}
      <Title headingLevel="h2" size="lg" style={{ marginBottom: '1rem' }}>
        Agent Card Index
      </Title>

      <Toolbar style={{ marginBottom: '1rem' }}>
        <ToolbarContent>
          <ToolbarItem>
            <TextInput
              type="search"
              aria-label="Search agents"
              placeholder="Search agents..."
              value={search}
              onChange={handleSearchChange}
            />
          </ToolbarItem>
        </ToolbarContent>
      </Toolbar>

      {indexLoading ? (
        <div data-testid="index-loading" style={{ textAlign: 'center', padding: '2rem' }}>
          <Spinner aria-label="Loading agent cards" />
        </div>
      ) : indexError ? (
        <Alert variant="danger" title="Error loading agent cards" data-testid="index-error" isInline>
          {indexError}
        </Alert>
      ) : cards.length === 0 ? (
        <Alert variant="info" title="No agent cards found" isInline isPlain />
      ) : (
        <>
          <Table aria-label="Agent cards table" data-testid="index-table">
            <Thead>
              <Tr>
                <Th screenReaderText="Expand" />
                <Th>Name</Th>
                <Th>Description</Th>
                <Th>Version</Th>
                <Th>Skills</Th>
                <Th>Actions</Th>
              </Tr>
            </Thead>
            <Tbody>
              {cards.map((card, idx) => (
                <Fragment key={card.url}>
                  <Tr>
                    <Td>
                      <Button
                        variant="plain"
                        size="sm"
                        data-testid={`expand-card-${idx}`}
                        onClick={() => toggleRow(idx)}
                        aria-label={expandedRows.has(idx) ? 'Collapse' : 'Expand'}
                      >
                        {expandedRows.has(idx) ? '\u25BC' : '\u25B6'}
                      </Button>
                    </Td>
                    <Td dataLabel="Name">{card.name}</Td>
                    <Td dataLabel="Description">{card.description}</Td>
                    <Td dataLabel="Version">
                      <Label>v{card.version}</Label>
                    </Td>
                    <Td dataLabel="Skills">{card.skills.length}</Td>
                    <Td>
                      <Button
                        variant="secondary"
                        size="sm"
                        onClick={() => copyToClipboard(JSON.stringify(card, null, 2))}
                      >
                        Copy Card JSON
                      </Button>
                    </Td>
                  </Tr>
                  {expandedRows.has(idx) && (
                    <Tr>
                      <Td colSpan={6}>
                        <CodeBlock>
                          <CodeBlockCode data-testid={`card-json-${idx}`}>
                            {JSON.stringify(card, null, 2)}
                          </CodeBlockCode>
                        </CodeBlock>
                      </Td>
                    </Tr>
                  )}
                </Fragment>
              ))}
            </Tbody>
          </Table>

          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginTop: '1rem' }}>
            <span data-testid="pagination-info">
              Page {currentPage} of {totalPages} ({total} total)
            </span>
            <div>
              <Button
                variant="secondary"
                size="sm"
                isDisabled={offset === 0}
                onClick={handlePrevPage}
                data-testid="prev-page"
                style={{ marginRight: '0.5rem' }}
              >
                Previous
              </Button>
              <Button
                variant="secondary"
                size="sm"
                isDisabled={offset + limit >= total}
                onClick={handleNextPage}
                data-testid="next-page"
              >
                Next
              </Button>
            </div>
          </div>
        </>
      )}

      {/* Section 3: External Registry Config */}
      <div data-testid="registry-config-section" style={{ marginTop: '2rem' }}>
        <Title headingLevel="h2" size="lg" style={{ marginBottom: '1rem' }}>
          External Registry Configuration
        </Title>
        <Alert variant="info" title="A2A Registry URL" isInline>
          The <code>A2A_REGISTRY_URL</code> environment variable controls external registry publishing.
          When configured, this registry will publish its agent cards to the specified external A2A registry.
          This setting is managed via server configuration and cannot be changed through the GUI.
        </Alert>
      </div>
    </div>
  );
}
