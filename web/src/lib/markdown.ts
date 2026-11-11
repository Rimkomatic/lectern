import DOMPurify from 'dompurify';
import { marked } from 'marked';

let hooked = false;

function hookLinks(): void {
  if (hooked) return;
  hooked = true;
  // Links in course notes open in a new tab and never get the opener.
  DOMPurify.addHook('afterSanitizeAttributes', (node) => {
    if (node.tagName === 'A' && node.getAttribute('href')) {
      node.setAttribute('target', '_blank');
      node.setAttribute('rel', 'noopener noreferrer');
    }
  });
}

/**
 * Renders Markdown to HTML that is safe to inject: the output of marked is
 * sanitised with DOMPurify, so scripts, event handlers and javascript: links
 * in a course's README cannot run.
 */
export function renderMarkdown(source: string): string {
  hookLinks();
  const html = marked.parse(source, { async: false, gfm: true, breaks: false }) as string;
  return DOMPurify.sanitize(html, { USE_PROFILES: { html: true } });
}
