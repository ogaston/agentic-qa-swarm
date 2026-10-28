import { useEffect, useRef } from 'react';

/**
 * Revelado al hacer scroll para el sistema editorial: observa los `.ed-reveal` dentro del nodo
 * referenciado y les añade `.ed-visible` cuando entran en el viewport. Sin IntersectionObserver
 * (jsdom, navegadores antiguos) se muestran de inmediato. Con prefers-reduced-motion la hoja
 * editorial los pinta visibles desde el principio y el bloque global de styles.css anula la
 * transición. No usa estilos en línea: la CSP es default-src 'self'.
 */
export function useReveal<T extends HTMLElement>() {
  const ref = useRef<T | null>(null);

  useEffect(() => {
    const root = ref.current;
    if (!root) return;

    const markVisible = (el: Element) => {
      el.classList.add('ed-visible');
      el.setAttribute('data-revealed', 'true');
    };

    const items = Array.from(root.querySelectorAll('.ed-reveal'));
    if (items.length === 0) return;

    if (typeof IntersectionObserver === 'undefined') {
      items.forEach(markVisible);
      return;
    }

    const observer = new IntersectionObserver(
      (entries) => {
        for (const entry of entries) {
          if (entry.isIntersecting || entry.boundingClientRect.bottom <= 0) {
            markVisible(entry.target);
            observer.unobserve(entry.target);
          }
        }
      },
      { threshold: 0.15 },
    );

    items.forEach((el) => {
      if (el.getAttribute('data-revealed') === 'true') {
        el.classList.add('ed-visible');
      } else {
        observer.observe(el);
      }
    });

    const revelarPasados = () => {
      for (const el of items) {
        if (!el.classList.contains('ed-visible') && el.getBoundingClientRect().bottom <= 0) {
          markVisible(el);
          observer.unobserve(el);
        }
      }
    };
    revelarPasados();
    window.addEventListener('scroll', revelarPasados, { passive: true });

    // Observa mutaciones en root para restaurar ed-visible si un re-render de React limpia las clases
    const mutObserver = new MutationObserver((mutations) => {
      for (const mutation of mutations) {
        if (mutation.type === 'attributes' && mutation.attributeName === 'class') {
          const target = mutation.target as HTMLElement;
          if (target.getAttribute('data-revealed') === 'true' && !target.classList.contains('ed-visible')) {
            target.classList.add('ed-visible');
          }
        }
      }
    });

    mutObserver.observe(root, {
      attributes: true,
      attributeFilter: ['class'],
      subtree: true,
    });

    return () => {
      observer.disconnect();
      mutObserver.disconnect();
      window.removeEventListener('scroll', revelarPasados);
    };
  }, []);

  return ref;
}
