import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { WebWidget } from "./web-widget";

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

describe("WebWidget", () => {
  it("renders an accessible conversation and sends on Enter", async () => {
    const onSend = vi.fn().mockResolvedValue(undefined);
    render(
      <WebWidget
        messages={[{ id: "m1", author: "assistant", content: "Hello" }]}
        onSend={onSend}
        locale="en"
      />
    );

    expect(screen.getByRole("log")).toHaveTextContent("Hello");
    const input = screen.getByRole("textbox", { name: "Message" });
    fireEvent.change(input, { target: { value: "Need help" } });
    fireEvent.keyDown(input, { key: "Enter" });
    await waitFor(() => expect(onSend).toHaveBeenCalledWith("Need help", []));
  });

  it("supports attachment selection and human handoff", async () => {
    const onSend = vi.fn().mockResolvedValue(undefined);
    const onUpload = vi
      .fn()
      .mockResolvedValue({ id: "file-1", name: "log.txt" });
    const onHandoff = vi.fn().mockResolvedValue(undefined);
    render(
      <WebWidget
        messages={[]}
        onAttachmentSelected={onUpload}
        onRequestHuman={onHandoff}
        onSend={onSend}
        theme="high-contrast"
      />
    );

    fireEvent.click(screen.getByRole("button", { name: "Attach a file" }));
    fireEvent.change(screen.getByTestId("widget-file-input"), {
      target: {
        files: [new File(["hello"], "log.txt", { type: "text/plain" })],
      },
    });
    await waitFor(() =>
      expect(screen.getByTestId("widget-attachments")).toHaveTextContent(
        "log.txt"
      )
    );
    fireEvent.click(screen.getByRole("button", { name: "Talk to a human" }));
    await waitFor(() => expect(onHandoff).toHaveBeenCalledTimes(1));
  });

  it("does not send empty messages and disables input during handoff", () => {
    const onSend = vi.fn();
    render(
      <WebWidget handoffActive messages={[]} onSend={onSend} locale="zh" />
    );
    expect(screen.getByRole("textbox", { name: "訊息" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "送出訊息" })).toBeDisabled();
    expect(onSend).not.toHaveBeenCalled();
  });
});
