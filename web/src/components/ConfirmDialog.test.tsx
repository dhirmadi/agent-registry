import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, it, expect, vi } from 'vitest';
import { ConfirmDialog } from './ConfirmDialog';

describe('ConfirmDialog', () => {
  const defaultProps = {
    isOpen: true,
    title: 'Delete Agent',
    message: 'Are you sure you want to delete this agent?',
    confirmText: 'Delete',
    onConfirm: vi.fn(),
    onCancel: vi.fn(),
    variant: 'danger' as const,
  };

  it('renders modal with title and message when open', () => {
    render(<ConfirmDialog {...defaultProps} />);
    expect(screen.getByText('Delete Agent')).toBeInTheDocument();
    expect(screen.getByText('Are you sure you want to delete this agent?')).toBeInTheDocument();
  });

  it('does not render content when closed', () => {
    render(<ConfirmDialog {...defaultProps} isOpen={false} />);
    expect(screen.queryByText('Delete Agent')).not.toBeInTheDocument();
  });

  it('calls onConfirm when confirm button is clicked', async () => {
    const onConfirm = vi.fn();
    const user = userEvent.setup();

    render(<ConfirmDialog {...defaultProps} onConfirm={onConfirm} />);
    await user.click(screen.getByTestId('confirm-button'));

    expect(onConfirm).toHaveBeenCalledTimes(1);
  });

  it('calls onCancel when cancel button is clicked', async () => {
    const onCancel = vi.fn();
    const user = userEvent.setup();

    render(<ConfirmDialog {...defaultProps} onCancel={onCancel} />);
    await user.click(screen.getByTestId('cancel-button'));

    expect(onCancel).toHaveBeenCalledTimes(1);
  });

  it('shows type-to-confirm input when resourceName is provided', () => {
    render(<ConfirmDialog {...defaultProps} resourceName="my-agent" />);
    expect(screen.getByText(/my-agent/)).toBeInTheDocument();
    expect(screen.getByTestId('confirm-name-input')).toBeInTheDocument();
  });

  it('disables confirm button until resource name is typed correctly', async () => {
    const onConfirm = vi.fn();
    const user = userEvent.setup();

    render(
      <ConfirmDialog {...defaultProps} onConfirm={onConfirm} resourceName="my-agent" />,
    );

    const confirmBtn = screen.getByTestId('confirm-button');
    expect(confirmBtn).toBeDisabled();

    const input = screen.getByTestId('confirm-name-input');
    await user.type(input, 'wrong-name');
    expect(confirmBtn).toBeDisabled();

    await user.clear(input);
    await user.type(input, 'my-agent');
    expect(confirmBtn).toBeEnabled();

    await user.click(confirmBtn);
    expect(onConfirm).toHaveBeenCalledTimes(1);
  });

  it('enables confirm button immediately when no resourceName', () => {
    render(<ConfirmDialog {...defaultProps} />);
    expect(screen.getByTestId('confirm-button')).toBeEnabled();
  });

  it('uses default confirm text when not provided', () => {
    render(
      <ConfirmDialog
        isOpen={true}
        title="Confirm"
        message="Are you sure?"
        onConfirm={vi.fn()}
        onCancel={vi.fn()}
      />,
    );
    expect(screen.getByTestId('confirm-button')).toHaveTextContent('Confirm');
  });
});
