import type { Metadata } from "next";
import "./globals.css";
import "@xterm/xterm/css/xterm.css";

export const metadata: Metadata = {
  title: "openCrow",
  description: "Self-hostable multi-device AI assistant",
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  // API_BASE_URL is a server-only env var (no NEXT_PUBLIC_ prefix).
  // It is read at request time — changing it only requires a container restart, not a rebuild.
  const apiBaseUrl = process.env.API_BASE_URL || "http://localhost:8080";

  return (
    <html lang="en" className="dark">
      <head>
        <meta name="x-api-base" content={apiBaseUrl} />
      </head>
      <body className="bg-surface text-on-surface">
        {children}
      </body>
    </html>
  );
}
