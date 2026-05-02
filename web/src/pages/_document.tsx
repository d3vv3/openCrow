// ─── openCrow _document ───
// HTML shell with dark theme enforced.

import { Html, Head, Main, NextScript } from "next/document";

export default function Document() {
  return (
    <Html lang="en" className="dark" suppressHydrationWarning>
      <Head>
        <meta name="description" content="Self-hostable multi-device AI assistant" />
        <link rel="icon" href="/crow.svg" type="image/svg+xml" />
      </Head>
      <body className="bg-surface text-on-surface">
        <Main />
        <NextScript />
      </body>
    </Html>
  );
}
