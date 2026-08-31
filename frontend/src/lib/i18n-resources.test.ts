import i18next from "i18next";
import { describe, expect, it } from "vitest";
import enUS from "@/locales/en-US.json";
import zhTW from "@/locales/zh-CN.json";

describe("runtime i18n resources", () => {
  it.each([
    [
      "en-US",
      enUS,
      "Agent Hub",
      "Public registration is for temporary use. It does not grant runtime execution.",
    ],
    [
      "zh-CN",
      zhTW,
      "Agent Hub",
      "公開註冊適合臨時使用，不會授予 runtime 執行權限。",
    ],
  ])("resolves nested Hub settings in %s", async (locale, resources, title, warning) => {
    const instance = i18next.createInstance();
    await instance.init({
      resources: { [locale]: { translation: resources } },
      lng: locale,
    });

    expect(instance.t("settings.hub.title")).toBe(title);
    expect(instance.t("settings.hub.public-warning")).toBe(warning);
    expect(instance.t("settings.hub.title")).not.toBe("settings.hub.title");
  });
});
