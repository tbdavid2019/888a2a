import { Paperclip, Send, UserRound } from "lucide-react";
import { type ChangeEvent, type KeyboardEvent, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { cn } from "@/lib/utils";

export type WidgetTheme = "light" | "dark" | "high-contrast";
export type WidgetAuthor = "visitor" | "assistant" | "human";

export interface WidgetAttachment {
  id: string;
  name: string;
  sizeBytes?: bigint;
}

export interface WidgetMessage {
  id: string;
  author: WidgetAuthor;
  content: string;
  createdAt?: Date;
  attachments?: WidgetAttachment[];
}

export interface WebWidgetProps {
  messages: WidgetMessage[];
  onSend: (
    content: string,
    attachments: WidgetAttachment[]
  ) => Promise<void> | void;
  onAttachmentSelected?: (
    file: File
  ) => Promise<WidgetAttachment | undefined> | WidgetAttachment | undefined;
  onRequestHuman?: () => Promise<void> | void;
  isLoading?: boolean;
  handoffActive?: boolean;
  theme?: WidgetTheme;
  locale?: "en" | "zh";
  title?: string;
}

const copy = {
  en: {
    subtitle: "Human and Agent support",
    messageLabel: "Message",
    placeholder: "Write a message…",
    send: "Send message",
    attach: "Attach a file",
    handoff: "Talk to a human",
    handoffActive: "A human is joining",
    sending: "Sending…",
    attachmentPending: "Uploading…",
    empty: "Start a conversation",
  },
  zh: {
    subtitle: "真人與 Agent 協作支援",
    messageLabel: "訊息",
    placeholder: "輸入訊息…",
    send: "送出訊息",
    attach: "附加檔案",
    handoff: "轉接真人",
    handoffActive: "真人即將加入",
    sending: "傳送中…",
    attachmentPending: "上傳中…",
    empty: "開始對話",
  },
} as const;

function authorLabel(author: WidgetAuthor, locale: "en" | "zh"): string {
  if (locale === "zh") {
    return author === "visitor" ? "你" : author === "human" ? "真人" : "Agent";
  }
  return author === "visitor" ? "You" : author === "human" ? "Human" : "Agent";
}

function formatTime(date: Date | undefined, locale: "en" | "zh"): string {
  if (!date) return "";
  return new Intl.DateTimeFormat(locale === "zh" ? "zh-TW" : "en-US", {
    hour: "numeric",
    minute: "2-digit",
  }).format(date);
}

export function WebWidget({
  messages,
  onSend,
  onAttachmentSelected,
  onRequestHuman,
  isLoading = false,
  handoffActive = false,
  theme = "light",
  locale = "en",
  title = "888a2a Support",
}: WebWidgetProps) {
  const { t: translate } = useTranslation();
  const labels = copy[locale];
  const [draft, setDraft] = useState("");
  const [attachments, setAttachments] = useState<WidgetAttachment[]>([]);
  const [uploading, setUploading] = useState(false);
  const [sending, setSending] = useState(false);
  const fileInputRef = useRef<HTMLInputElement>(null);

  const send = async () => {
    const content = draft.trim();
    if ((!content && attachments.length === 0) || sending || uploading) return;
    setSending(true);
    try {
      await onSend(content, attachments);
      setDraft("");
      setAttachments([]);
    } finally {
      setSending(false);
    }
  };

  const selectAttachment = async (event: ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    event.target.value = "";
    if (!file || !onAttachmentSelected) return;
    setUploading(true);
    try {
      const attachment = await onAttachmentSelected(file);
      if (attachment) setAttachments((current) => [...current, attachment]);
    } finally {
      setUploading(false);
    }
  };

  const handleKeyDown = (event: KeyboardEvent<HTMLTextAreaElement>) => {
    if (event.key === "Enter" && !event.shiftKey) {
      event.preventDefault();
      void send();
    }
  };

  return (
    <section
      aria-label={title}
      className={cn(
        "flex h-[min(680px,calc(100vh-2rem))] w-full max-w-md flex-col overflow-hidden rounded-2xl border shadow-xl",
        theme === "dark" && "border-zinc-700 bg-zinc-950 text-zinc-50",
        theme === "high-contrast" &&
          "border-2 border-black bg-white text-black",
        theme === "light" && "border-control-border bg-background text-main"
      )}
      data-testid="web-widget"
    >
      <header className="flex items-start justify-between gap-4 border-b border-current/10 px-5 py-4">
        <div className="min-w-0">
          <h2 className="truncate text-base font-semibold">{title}</h2>
          <p className="mt-1 text-xs opacity-70">{labels.subtitle}</p>
        </div>
        {onRequestHuman && (
          <Button
            aria-label={handoffActive ? labels.handoffActive : labels.handoff}
            disabled={handoffActive}
            onClick={() => void onRequestHuman()}
            size="sm"
            variant={theme === "high-contrast" ? "outline" : "ghost"}
          >
            <UserRound aria-hidden="true" className="size-3.5" />
            {handoffActive ? labels.handoffActive : labels.handoff}
          </Button>
        )}
      </header>

      <div
        aria-busy={isLoading}
        aria-live="polite"
        className="flex-1 space-y-4 overflow-y-auto px-4 py-5"
        data-testid="widget-messages"
        role="log"
      >
        {messages.length === 0 && !isLoading && (
          <p className="py-12 text-center text-sm opacity-60">{labels.empty}</p>
        )}
        {messages.map((message) => {
          const visitor = message.author === "visitor";
          return (
            <article
              className={cn("flex gap-2", visitor && "flex-row-reverse")}
              key={message.id}
            >
              <div
                aria-hidden="true"
                className={cn(
                  "mt-1 flex size-7 shrink-0 items-center justify-center rounded-full text-xs font-semibold",
                  visitor
                    ? "bg-accent text-accent-text"
                    : "bg-control-bg text-control"
                )}
              >
                {visitor ? "Y" : "A"}
              </div>
              <div className={cn("max-w-[82%]", visitor && "text-right")}>
                <div className="mb-1 flex items-center gap-2 text-[11px] opacity-65">
                  <span>{authorLabel(message.author, locale)}</span>
                  <time dateTime={message.createdAt?.toISOString()}>
                    {formatTime(message.createdAt, locale)}
                  </time>
                </div>
                <div
                  className={cn(
                    "rounded-xl px-3 py-2 text-sm leading-5",
                    visitor ? "bg-accent text-accent-text" : "bg-control-bg"
                  )}
                >
                  {message.content}
                  {message.attachments?.map((attachment) => (
                    <Badge
                      className="mt-2 mr-1"
                      key={attachment.id}
                      variant="secondary"
                    >
                      <Paperclip aria-hidden="true" className="mr-1 size-3" />
                      {attachment.name}
                    </Badge>
                  ))}
                </div>
              </div>
            </article>
          );
        })}
        {isLoading && (
          <p className="text-center text-xs opacity-60">
            {translate("common.loading") || labels.sending}
          </p>
        )}
      </div>

      {attachments.length > 0 && (
        <div
          className="flex flex-wrap gap-1 border-t border-current/10 px-4 pt-3"
          data-testid="widget-attachments"
        >
          {attachments.map((attachment) => (
            <Badge key={attachment.id} variant="secondary">
              <Paperclip aria-hidden="true" className="mr-1 size-3" />
              {attachment.name}
            </Badge>
          ))}
        </div>
      )}

      <footer className="border-t border-current/10 p-4">
        <label className="sr-only" htmlFor="web-widget-message">
          {labels.messageLabel}
        </label>
        <div className="flex items-end gap-2">
          <Textarea
            aria-label={labels.messageLabel}
            className="min-h-11 resize-none"
            disabled={sending || handoffActive}
            id="web-widget-message"
            onChange={(event) => setDraft(event.target.value)}
            onKeyDown={handleKeyDown}
            placeholder={labels.placeholder}
            rows={2}
            value={draft}
          />
          <div className="flex shrink-0 flex-col gap-1">
            <input
              accept="image/*,.pdf,.txt,.csv"
              className="sr-only"
              data-testid="widget-file-input"
              onChange={(event) => void selectAttachment(event)}
              ref={fileInputRef}
              type="file"
            />
            <Button
              aria-label={labels.attach}
              disabled={
                !onAttachmentSelected || uploading || sending || handoffActive
              }
              onClick={() => fileInputRef.current?.click()}
              size="sm"
              variant="ghost"
            >
              <Paperclip aria-hidden="true" className="size-4" />
            </Button>
            <Button
              aria-label={sending ? labels.sending : labels.send}
              disabled={
                (!draft.trim() && attachments.length === 0) ||
                sending ||
                uploading ||
                handoffActive
              }
              onClick={() => void send()}
              size="sm"
            >
              <Send aria-hidden="true" className="size-4" />
            </Button>
          </div>
        </div>
        <p className="mt-2 text-[11px] opacity-60">
          Shift + Enter for a new line
        </p>
      </footer>
    </section>
  );
}
