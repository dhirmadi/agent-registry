import { Button, Label } from '@patternfly/react-core';

export interface TimelineVersion {
  version: number;
  created_by: string;
  created_at: string;
  is_current: boolean;
}

interface VersionTimelineProps {
  versions: TimelineVersion[];
  onRollback?: (version: number) => void;
  readOnly?: boolean;
}

export function VersionTimeline({
  versions,
  onRollback,
  readOnly = false,
}: VersionTimelineProps) {
  const sorted = [...versions].sort((a, b) => b.version - a.version);

  return (
    <div style={{ position: 'relative', paddingLeft: '1.5rem' }}>
      <div
        style={{
          position: 'absolute',
          left: '0.5rem',
          top: 0,
          bottom: 0,
          width: '2px',
          backgroundColor: 'var(--pf-t--global--border--color--default, #d2d2d2)',
        }}
      />
      {sorted.map((v) => (
        <div
          key={v.version}
          style={{
            position: 'relative',
            paddingBottom: '1rem',
            paddingLeft: '1rem',
          }}
          data-testid={`version-${v.version}`}
        >
          <div
            style={{
              position: 'absolute',
              left: '-1.15rem',
              top: '0.25rem',
              width: '0.75rem',
              height: '0.75rem',
              borderRadius: '50%',
              backgroundColor: v.is_current
                ? 'var(--pf-t--global--color--status--success--default, #3e8635)'
                : 'var(--pf-t--global--border--color--default, #d2d2d2)',
            }}
          />
          <div style={{ display: 'flex', alignItems: 'center', gap: '0.5rem', flexWrap: 'wrap' }}>
            <strong>v{v.version}</strong>
            {v.is_current && <Label color="green">Current</Label>}
            <span style={{ color: 'var(--pf-t--global--text--color--subtle, #6a6e73)' }}>
              by {v.created_by} on {new Date(v.created_at).toLocaleDateString()}
            </span>
            {!v.is_current && !readOnly && onRollback && (
              <Button
                variant="link"
                size="sm"
                onClick={() => onRollback(v.version)}
                data-testid={`rollback-${v.version}`}
              >
                Rollback
              </Button>
            )}
          </div>
        </div>
      ))}
    </div>
  );
}
