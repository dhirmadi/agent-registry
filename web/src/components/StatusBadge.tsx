import { Label } from '@patternfly/react-core';

interface StatusBadgeProps {
  status: string;
}

const greenStatuses = new Set(['active', 'enabled', 'healthy']);
const redStatuses = new Set(['inactive', 'disabled', 'unhealthy']);

export function StatusBadge({ status }: StatusBadgeProps) {
  const normalized = status.toLowerCase();
  let color: 'green' | 'red' | 'grey' = 'grey';

  if (greenStatuses.has(normalized)) {
    color = 'green';
  } else if (redStatuses.has(normalized)) {
    color = 'red';
  }

  return <Label color={color}>{status}</Label>;
}
