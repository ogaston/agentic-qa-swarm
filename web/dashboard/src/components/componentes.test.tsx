import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { Badge } from './Badge';
import { Card } from './Card';
import { Kpi } from './Kpi';
import { PageHeader } from './PageHeader';

describe('componentes base del mockup', () => {
  it('PageHeader: título, descripción y metadatos en la franja del mockup', () => {
    const { container } = render(
      <PageHeader
        titulo="Inbox"
        descripcion="Notificaciones pendientes"
        meta={[{ etiqueta: 'Entorno', valor: 'Listo' }]}
      />,
    );
    expect(screen.getByRole('heading', { name: 'Inbox' })).toBeTruthy();
    expect(screen.getByText('Notificaciones pendientes')).toBeTruthy();
    expect(screen.getByText('Entorno').className).toBe('az-meta-label');
    expect(container.querySelector('.az-page-header .az-header-meta .az-meta-item')).not.toBeNull();
  });

  it('Badge: tono como clase y punto opcional con el texto visible', () => {
    const { container } = render(
      <Badge tono="ok" punto>
        hecha
      </Badge>,
    );
    const insignia = screen.getByText('hecha');
    expect(insignia.className).toBe('badge badge-ok');
    expect(container.querySelector('.badge-dot')).not.toBeNull();
  });

  it('Badge: variante plana conserva el punto y el texto coloreado sin fondo ni borde', () => {
    const { container } = render(
      <Badge tono="warn" punto plano>
        pending
      </Badge>,
    );
    const insignia = screen.getByText('pending');
    expect(insignia.className).toBe('badge badge-warn badge-plano');
    expect(container.querySelector('.badge-dot')).not.toBeNull();
  });

  it('Card: relleno del mockup por defecto y sin relleno si pad es false', () => {
    const { container, rerender } = render(<Card>contenido</Card>);
    expect(container.firstElementChild?.className).toBe('card card-pad');
    rerender(<Card pad={false}>contenido</Card>);
    expect(container.firstElementChild?.className).toBe('card');
  });

  it('Kpi: valor, etiqueta y barra acotada a 0–100', () => {
    const { container } = render(<Kpi etiqueta="Hallazgos" valor="7" barra={150} />);
    expect(screen.getByText('Hallazgos').className).toBe('kpi-label');
    expect(screen.getByText('7').className).toBe('kpi-val');
    const relleno = container.querySelector('.kpi-bar-fill') as HTMLElement | null;
    expect(relleno?.style.width).toBe('100%');
  });
});
