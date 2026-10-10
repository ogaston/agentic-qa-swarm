import { act, render, screen } from '@testing-library/react';
import { describe, expect, it, vi, afterEach } from 'vitest';
import { ApiError } from './client';
import { ApiErrorNotice } from './errors';

afterEach(() => {
  vi.useRealTimers();
});

describe('ApiErrorNotice', () => {
  it('429 muestra la cuenta atrás de Retry-After', async () => {
    vi.useFakeTimers();
    render(
      <ApiErrorNotice
        error={new ApiError({ status: 429, code: 'rate_limited', message: 'x', requestId: 'req-abc12345', retryAfter: 5 })}
      />,
    );
    expect(screen.getByText(/espera 5 s/)).toBeTruthy();
    await act(async () => {
      vi.advanceTimersByTime(2000);
    });
    expect(screen.getByText(/espera 3 s/)).toBeTruthy();
  });

  it('503 muestra servicio no disponible y el Retry-After si viene', () => {
    render(
      <ApiErrorNotice
        error={new ApiError({ status: 503, code: 'unavailable', message: 'x', requestId: 'req-abc12345', retryAfter: 10 })}
      />,
    );
    expect(screen.getByText(/Servicio no disponible/)).toBeTruthy();
    expect(screen.getByText(/10 s/)).toBeTruthy();
  });

  it('otros errores muestran code, message y requestId del cuerpo', () => {
    render(
      <ApiErrorNotice
        error={new ApiError({ status: 400, code: 'invalid_input', message: 'Dato inválido', requestId: 'req-abc12345' })}
      />,
    );
    expect(screen.getByText(/invalid_input/)).toBeTruthy();
    expect(screen.getByText(/Dato inválido/)).toBeTruthy();
    expect(screen.getByText(/req-abc12345/)).toBeTruthy();
  });
});
