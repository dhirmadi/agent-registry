import { render, screen } from '@testing-library/react';
import { describe, it, expect } from 'vitest';
import { DiffViewer } from './DiffViewer';

describe('DiffViewer', () => {
  it('renders both sides with labels', () => {
    render(
      <DiffViewer
        oldText="hello"
        newText="hello"
        oldLabel="Old Version"
        newLabel="New Version"
      />,
    );

    expect(screen.getByText('Old Version')).toBeInTheDocument();
    expect(screen.getByText('New Version')).toBeInTheDocument();
    expect(screen.getByTestId('diff-left')).toBeInTheDocument();
    expect(screen.getByTestId('diff-right')).toBeInTheDocument();
  });

  it('shows unchanged lines without highlighting', () => {
    render(<DiffViewer oldText="line one" newText="line one" />);

    const left = screen.getByTestId('diff-left');
    const right = screen.getByTestId('diff-right');

    const leftLines = left.querySelectorAll('[data-diff-type="unchanged"]');
    const rightLines = right.querySelectorAll('[data-diff-type="unchanged"]');

    expect(leftLines.length).toBeGreaterThan(0);
    expect(rightLines.length).toBeGreaterThan(0);
    expect(leftLines[0].textContent).toBe('line one');
    expect(rightLines[0].textContent).toBe('line one');
  });

  it('highlights removed lines on the left', () => {
    render(<DiffViewer oldText={"line one\nremoved line"} newText="line one" />);

    const left = screen.getByTestId('diff-left');
    const removedLines = left.querySelectorAll('[data-diff-type="removed"]');

    expect(removedLines.length).toBeGreaterThan(0);
    expect(removedLines[0].textContent).toBe('removed line');
  });

  it('highlights added lines on the right', () => {
    render(<DiffViewer oldText="line one" newText={"line one\nadded line"} />);

    const right = screen.getByTestId('diff-right');
    const addedLines = right.querySelectorAll('[data-diff-type="added"]');

    expect(addedLines.length).toBeGreaterThan(0);
    expect(addedLines[0].textContent).toBe('added line');
  });

  it('highlights changed lines on both sides', () => {
    render(<DiffViewer oldText="old content" newText="new content" />);

    const left = screen.getByTestId('diff-left');
    const right = screen.getByTestId('diff-right');

    const removedLines = left.querySelectorAll('[data-diff-type="removed"]');
    const addedLines = right.querySelectorAll('[data-diff-type="added"]');

    expect(removedLines.length).toBeGreaterThan(0);
    expect(addedLines.length).toBeGreaterThan(0);
    expect(removedLines[0].textContent).toBe('old content');
    expect(addedLines[0].textContent).toBe('new content');
  });

  it('uses default labels when not provided', () => {
    render(<DiffViewer oldText="a" newText="b" />);

    expect(screen.getByText('Previous')).toBeInTheDocument();
    expect(screen.getByText('Current')).toBeInTheDocument();
  });

  it('handles multi-line diffs', () => {
    const oldText = 'line 1\nline 2\nline 3';
    const newText = 'line 1\nmodified\nline 3';

    render(<DiffViewer oldText={oldText} newText={newText} />);

    const left = screen.getByTestId('diff-left');
    const right = screen.getByTestId('diff-right');

    // line 1 and line 3 should be unchanged
    const leftUnchanged = left.querySelectorAll('[data-diff-type="unchanged"]');
    const rightUnchanged = right.querySelectorAll('[data-diff-type="unchanged"]');

    const leftUnchangedTexts = Array.from(leftUnchanged).map((el) => el.textContent);
    const rightUnchangedTexts = Array.from(rightUnchanged).map((el) => el.textContent);

    expect(leftUnchangedTexts).toContain('line 1');
    expect(leftUnchangedTexts).toContain('line 3');
    expect(rightUnchangedTexts).toContain('line 1');
    expect(rightUnchangedTexts).toContain('line 3');

    // line 2 / modified should be changed
    const removedLines = left.querySelectorAll('[data-diff-type="removed"]');
    const addedLines = right.querySelectorAll('[data-diff-type="added"]');

    expect(removedLines.length).toBeGreaterThan(0);
    expect(addedLines.length).toBeGreaterThan(0);
  });

  it('handles empty strings', () => {
    render(<DiffViewer oldText="" newText="" />);
    expect(screen.getByTestId('diff-left')).toBeInTheDocument();
    expect(screen.getByTestId('diff-right')).toBeInTheDocument();
  });
});
