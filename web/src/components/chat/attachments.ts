import type { CreateMessageAttachmentRequest, MessageAttachmentDTO } from "@/lib/api-types";

export type PickedAttachmentFile = { file: File; dataUrl: string };

export function toAttachmentPayload(
  files: PickedAttachmentFile[],
): CreateMessageAttachmentRequest[] {
  return files.map(({ file, dataUrl }) => ({
    fileName: file.name,
    mimeType: file.type || "application/octet-stream",
    sizeBytes: file.size,
    dataUrl,
  }));
}

export function toOptimisticAttachments(files: PickedAttachmentFile[]): MessageAttachmentDTO[] {
  return files.map(({ file, dataUrl }, index) => ({
    id: `temp-attachment-${Date.now()}-${index}-${Math.random().toString(36).slice(2, 8)}`,
    fileName: file.name,
    mimeType: file.type || "application/octet-stream",
    sizeBytes: file.size,
    dataUrl,
    createdAt: new Date().toISOString(),
  }));
}

export function buildCompletionMessage(text: string, files: PickedAttachmentFile[]): string {
  const baseText = text.trim();
  const images = files.filter((f) => {
    const kind = f.file.type.trim().toLowerCase();
    return kind.startsWith("image/") && f.dataUrl.startsWith("data:image/");
  });
  const nonImages = files.filter((f) => !images.includes(f));

  const parts: string[] = [];
  if (baseText) parts.push(baseText);

  if (images.length > 0) {
    const imageMarkdown = images
      .map(({ file, dataUrl }) => `![${file.name}](${dataUrl})`)
      .join("\n");
    parts.push(`Attached images:\n${imageMarkdown}`);
  }

  if (nonImages.length > 0) {
    const fileList = nonImages
      .map(({ file }) => {
        const size = formatAttachmentSize(file.size);
        const type = file.type || "application/octet-stream";
        return `- ${file.name} (${type}${size ? `, ${size}` : ""})`;
      })
      .join("\n");
    parts.push(`Attached files:\n${fileList}`);

    const fileMarkdown = nonImages
      .map(({ file, dataUrl }) => `- [${file.name}](${dataUrl})`)
      .join("\n");
    parts.push(`Attached files as data URLs:\n${fileMarkdown}`);

    const attachmentContextBlocks = nonImages
      .map(({ file, dataUrl }) => buildAttachmentContextBlock(file.name, file.type, dataUrl))
      .filter((v): v is string => Boolean(v));
    if (attachmentContextBlocks.length > 0) {
      parts.push(`Attachment contents for analysis:\n\n${attachmentContextBlocks.join("\n\n")}`);
    }
  }

  return parts.join("\n\n");
}

function buildAttachmentContextBlock(
  fileName: string,
  mimeType: string,
  dataUrl: string,
): string | null {
  const type = (mimeType || "application/octet-stream").toLowerCase();
  const parsed = parseDataURL(dataUrl);
  if (!parsed) return null;

  const decodedText = decodeTextContent(type, parsed);
  if (decodedText) {
    const clipped = clipText(decodedText, 12000);
    return `### ${fileName}\n\n\`\`\`text\n${clipped}\n\`\`\``;
  }

  // For binary formats (e.g. PDFs), we now pass the full data URL as a
  // dedicated markdown link in buildCompletionMessage(). Avoid duplicating
  // massive base64 blocks in plain text here.
  return `### ${fileName}\n\nBinary attachment (${type}). Use the attached data-url file part for analysis.`;
}

function parseDataURL(
  dataUrl: string,
): { mime: string; isBase64: boolean; payload: string } | null {
  const match = dataUrl.match(/^data:([^;,]+)?(?:;charset=[^;,]+)?(;base64)?,(.*)$/is);
  if (!match) return null;
  return {
    mime: (match[1] || "application/octet-stream").toLowerCase(),
    isBase64: Boolean(match[2]),
    payload: match[3] || "",
  };
}

function decodeTextContent(
  mimeType: string,
  parsed: { mime: string; isBase64: boolean; payload: string },
): string {
  const textLikeMime =
    mimeType.startsWith("text/") ||
    mimeType === "application/json" ||
    mimeType === "application/xml" ||
    mimeType === "text/csv" ||
    mimeType === "application/javascript";
  if (!textLikeMime) return "";

  try {
    if (parsed.isBase64) {
      const binary = atob(parsed.payload);
      const bytes = Uint8Array.from(binary, (c) => c.charCodeAt(0));
      return new TextDecoder("utf-8", { fatal: false }).decode(bytes);
    }
    return decodeURIComponent(parsed.payload);
  } catch {
    return "";
  }
}

function clipText(value: string, limit: number): string {
  if (value.length <= limit) return value;
  return `${value.slice(0, limit)}\n...[truncated ${value.length - limit} chars]`;
}

export function formatAttachmentSize(sizeBytes?: number): string {
  const n = Number(sizeBytes ?? 0);
  if (!Number.isFinite(n) || n <= 0) return "";
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  return `${(n / (1024 * 1024)).toFixed(1)} MB`;
}
