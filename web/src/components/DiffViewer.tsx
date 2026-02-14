interface DiffViewerProps {
  oldText: string;
  newText: string;
  oldLabel?: string;
  newLabel?: string;
}

interface DiffLine {
  type: 'unchanged' | 'added' | 'removed';
  text: string;
}

function computeDiff(oldLines: string[], newLines: string[]): { left: DiffLine[]; right: DiffLine[] } {
  const left: DiffLine[] = [];
  const right: DiffLine[] = [];

  const maxLen = Math.max(oldLines.length, newLines.length);
  // Simple line-by-line comparison
  let oi = 0;
  let ni = 0;

  while (oi < oldLines.length || ni < newLines.length) {
    if (oi < oldLines.length && ni < newLines.length) {
      if (oldLines[oi] === newLines[ni]) {
        left.push({ type: 'unchanged', text: oldLines[oi] });
        right.push({ type: 'unchanged', text: newLines[ni] });
        oi++;
        ni++;
      } else {
        // Check if the old line appears later in new (was moved)
        const newIdx = newLines.indexOf(oldLines[oi], ni);
        const oldIdx = oldLines.indexOf(newLines[ni], oi);

        if (oldIdx === -1 && newIdx === -1) {
          // Both changed
          left.push({ type: 'removed', text: oldLines[oi] });
          right.push({ type: 'added', text: newLines[ni] });
          oi++;
          ni++;
        } else if (newIdx !== -1 && (oldIdx === -1 || newIdx - ni <= oldIdx - oi)) {
          // Lines were added in new
          while (ni < newIdx) {
            left.push({ type: 'unchanged', text: '' });
            right.push({ type: 'added', text: newLines[ni] });
            ni++;
          }
        } else {
          // Lines were removed from old
          while (oi < oldIdx) {
            left.push({ type: 'removed', text: oldLines[oi] });
            right.push({ type: 'unchanged', text: '' });
            oi++;
          }
        }
      }
    } else if (oi < oldLines.length) {
      left.push({ type: 'removed', text: oldLines[oi] });
      right.push({ type: 'unchanged', text: '' });
      oi++;
    } else {
      left.push({ type: 'unchanged', text: '' });
      right.push({ type: 'added', text: newLines[ni] });
      ni++;
    }

    // Safety guard against infinite loops
    if (left.length > maxLen * 3) break;
  }

  return { left, right };
}

const lineStyle: Record<DiffLine['type'], React.CSSProperties> = {
  unchanged: {},
  added: { backgroundColor: 'var(--pf-t--global--color--status--success--default, #e6ffe6)' },
  removed: { backgroundColor: 'var(--pf-t--global--color--status--danger--default, #ffe6e6)' },
};

export function DiffViewer({
  oldText,
  newText,
  oldLabel = 'Previous',
  newLabel = 'Current',
}: DiffViewerProps) {
  const oldLines = oldText.split('\n');
  const newLines = newText.split('\n');
  const { left, right } = computeDiff(oldLines, newLines);

  return (
    <div style={{ display: 'flex', gap: '1rem', fontFamily: 'monospace', fontSize: '0.875rem' }}>
      <div style={{ flex: 1, overflow: 'auto' }} data-testid="diff-left">
        <div style={{ fontWeight: 600, marginBottom: '0.5rem' }}>{oldLabel}</div>
        {left.map((line, i) => (
          <div
            key={i}
            style={{ ...lineStyle[line.type], padding: '2px 4px', whiteSpace: 'pre-wrap', minHeight: '1.25em' }}
            data-diff-type={line.type}
          >
            {line.text}
          </div>
        ))}
      </div>
      <div style={{ flex: 1, overflow: 'auto' }} data-testid="diff-right">
        <div style={{ fontWeight: 600, marginBottom: '0.5rem' }}>{newLabel}</div>
        {right.map((line, i) => (
          <div
            key={i}
            style={{ ...lineStyle[line.type], padding: '2px 4px', whiteSpace: 'pre-wrap', minHeight: '1.25em' }}
            data-diff-type={line.type}
          >
            {line.text}
          </div>
        ))}
      </div>
    </div>
  );
}
