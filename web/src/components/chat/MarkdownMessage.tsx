// chat/MarkdownMessage.tsx — Markdown renderer with custom styling for chat messages.
"use client";

import ReactMarkdown, { defaultUrlTransform } from "react-markdown";

function safeMarkdownUrlTransform(url: string) {
  if (/^data:image\//i.test(url)) return url;
  return defaultUrlTransform(url);
}

function MarkdownImage({ src, alt }: { src?: string | Blob | null; alt?: string | null }) {
  if (typeof src !== "string" || !src) return null;
  return (
    <img src={src} alt={alt ?? ""} className="mt-2 max-w-full rounded-lg max-h-64 object-contain" />
  );
}

function linkifyPlainUrls(content: string): string {
  return content.replace(
    /(^|[\s(])((?:https?:\/\/)[^\s<]+)/gi,
    (_, prefix: string, raw: string) => {
      let url = raw;
      let trailing = "";
      while (/[),.;!?]$/.test(url)) {
        trailing = url.slice(-1) + trailing;
        url = url.slice(0, -1);
      }
      return `${prefix}<${url}>${trailing}`;
    },
  );
}

export function MarkdownMessage({
  content,
  compact = false,
}: {
  content: string;
  compact?: boolean;
}) {
  return (
    <ReactMarkdown
      urlTransform={safeMarkdownUrlTransform}
      components={{
        p: ({ children }) => (
          <p className={compact ? "mb-1.5 last:mb-0" : "mb-2 last:mb-0"}>{children}</p>
        ),
        code: ({ children, className }) => {
          const isBlock = className?.includes("language-");
          return isBlock ? (
            <pre
              className={`${compact ? "bg-black/20" : "bg-black/30"} rounded p-2 overflow-x-auto my-2 text-xs font-mono`}
            >
              <code>{children}</code>
            </pre>
          ) : (
            <code
              className={`${compact ? "bg-black/20" : "bg-black/30"} rounded px-1 py-0.5 text-xs font-mono`}
            >
              {children}
            </code>
          );
        },
        ul: ({ children }) => (
          <ul
            className={
              compact ? "list-disc pl-4 mb-1.5 space-y-0.5" : "list-disc pl-4 mb-2 space-y-1"
            }
          >
            {children}
          </ul>
        ),
        ol: ({ children }) => (
          <ol
            className={
              compact ? "list-decimal pl-4 mb-1.5 space-y-0.5" : "list-decimal pl-4 mb-2 space-y-1"
            }
          >
            {children}
          </ol>
        ),
        li: ({ children }) => <li>{children}</li>,
        h1: ({ children }) => <h1 className="text-base font-bold mb-2">{children}</h1>,
        h2: ({ children }) => <h2 className="text-sm font-semibold mb-1.5">{children}</h2>,
        h3: ({ children }) => <h3 className="text-sm font-medium mb-1">{children}</h3>,
        strong: ({ children }) => <strong className="font-semibold">{children}</strong>,
        em: ({ children }) => <em className="italic">{children}</em>,
        blockquote: ({ children }) => (
          <blockquote className="border-l-2 border-white/20 pl-3 italic opacity-80">
            {children}
          </blockquote>
        ),
        a: ({ href, children }) => (
          <a
            href={href}
            target="_blank"
            rel="noopener"
            className="text-white/90 underline decoration-white/50"
          >
            {children}
          </a>
        ),
        img: ({ src, alt }) => <MarkdownImage src={src} alt={alt} />,
      }}
    >
      {linkifyPlainUrls(content)}
    </ReactMarkdown>
  );
}
