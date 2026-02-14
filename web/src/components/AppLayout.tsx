import { useCallback } from 'react';
import { Outlet, NavLink, useLocation } from 'react-router-dom';
import {
  Button,
  Label,
  Masthead,
  MastheadBrand,
  MastheadContent,
  MastheadMain,
  Nav,
  NavGroup,
  NavItem,
  Page,
  PageSection,
  PageSidebar,
  PageSidebarBody,
  Toolbar,
  ToolbarContent,
  ToolbarGroup,
  ToolbarItem,
} from '@patternfly/react-core';
import { useAuth } from '../auth/AuthContext';

const roleLabelColor: Record<string, 'blue' | 'green' | 'grey'> = {
  admin: 'blue',
  editor: 'green',
  viewer: 'grey',
};

export function AppLayout() {
  const { user, logout } = useAuth();
  const location = useLocation();

  const isActive = useCallback(
    (path: string) => location.pathname === path,
    [location.pathname],
  );

  const headerToolbar = (
    <Toolbar id="masthead-toolbar" isFullHeight isStatic>
      <ToolbarContent>
        <ToolbarGroup align={{ default: 'alignRight' }}>
          {user && (
            <ToolbarItem>
              <span data-testid="user-display">
                {user.display_name}{' '}
                <Label color={roleLabelColor[user.role] ?? 'grey'}>
                  {user.role}
                </Label>
              </span>
            </ToolbarItem>
          )}
          <ToolbarItem>
            <Button variant="link" onClick={logout}>
              Logout
            </Button>
          </ToolbarItem>
        </ToolbarGroup>
      </ToolbarContent>
    </Toolbar>
  );

  const masthead = (
    <Masthead>
      <MastheadMain>
        <MastheadBrand data-testid="app-title">
          <span style={{ fontSize: '1.25rem', fontWeight: 600 }}>
            Agentic Registry
          </span>
        </MastheadBrand>
      </MastheadMain>
      <MastheadContent>{headerToolbar}</MastheadContent>
    </Masthead>
  );

  const navItems = (
    <Nav aria-label="Primary navigation">
      <NavGroup aria-label="Overview" title="">
        <NavItem isActive={isActive('/')}>
          <NavLink to="/">Dashboard</NavLink>
        </NavItem>
      </NavGroup>
      <NavGroup title="Resources">
        <NavItem isActive={isActive('/agents')}>
          <NavLink to="/agents">Agents</NavLink>
        </NavItem>
        <NavItem isActive={isActive('/prompts')}>
          <NavLink to="/prompts">Prompts</NavLink>
        </NavItem>
        <NavItem isActive={isActive('/mcp-servers')}>
          <NavLink to="/mcp-servers">MCP Servers</NavLink>
        </NavItem>
      </NavGroup>
      <NavGroup title="Rules">
        <NavItem isActive={isActive('/trust-rules')}>
          <NavLink to="/trust-rules">Trust Rules</NavLink>
        </NavItem>
        <NavItem isActive={isActive('/trigger-rules')}>
          <NavLink to="/trigger-rules">Trigger Rules</NavLink>
        </NavItem>
      </NavGroup>
      <NavGroup title="Configuration">
        <NavItem isActive={isActive('/model-config')}>
          <NavLink to="/model-config">Model Config</NavLink>
        </NavItem>
        <NavItem isActive={isActive('/context-config')}>
          <NavLink to="/context-config">Context Config</NavLink>
        </NavItem>
        <NavItem isActive={isActive('/signal-polling')}>
          <NavLink to="/signal-polling">Signal Polling</NavLink>
        </NavItem>
      </NavGroup>
      <NavGroup title="System">
        <NavItem isActive={isActive('/webhooks')}>
          <NavLink to="/webhooks">Webhooks</NavLink>
        </NavItem>
        <NavItem isActive={isActive('/api-keys')}>
          <NavLink to="/api-keys">API Keys</NavLink>
        </NavItem>
        {user?.role === 'admin' && (
          <NavItem isActive={isActive('/users')}>
            <NavLink to="/users">Users</NavLink>
          </NavItem>
        )}
        <NavItem isActive={isActive('/audit-log')}>
          <NavLink to="/audit-log">Audit Log</NavLink>
        </NavItem>
      </NavGroup>
      <NavGroup aria-label="Account" title="">
        <NavItem isActive={isActive('/my-account')}>
          <NavLink to="/my-account">My Account</NavLink>
        </NavItem>
      </NavGroup>
    </Nav>
  );

  const sidebar = (
    <PageSidebar>
      <PageSidebarBody>{navItems}</PageSidebarBody>
    </PageSidebar>
  );

  return (
    <Page header={masthead} sidebar={sidebar}>
      <PageSection>
        <Outlet />
      </PageSection>
    </Page>
  );
}
