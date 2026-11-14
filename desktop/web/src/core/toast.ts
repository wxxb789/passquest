// Transient status line. Text only: the message is set with textContent, never parsed as HTML.
let timer: ReturnType<typeof setTimeout> | undefined;

export function toast(message: string): void {
  document.querySelector('.toast')?.remove();
  const t = document.createElement('div');
  t.className = 'toast';
  t.setAttribute('role', 'status');
  t.textContent = message;
  document.body.appendChild(t);
  clearTimeout(timer);
  timer = setTimeout(() => t.remove(), 2600);
}
