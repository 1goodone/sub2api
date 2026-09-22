import { readFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

import { describe, expect, it } from "vitest";

import en from "@/i18n/locales/en/admin/overview";
import zh from "@/i18n/locales/zh/admin/overview";

const currentDir = dirname(fileURLToPath(import.meta.url));
const source = readFileSync(resolve(currentDir, "../GroupsView.vue"), "utf8");

describe("GroupsView cache-read reduction", () => {
  it("binds bounded percentage inputs in both create and edit forms", () => {
    expect(
      source.match(/v-model\.number="(?:create|edit)Form\.cache_read_reduction_percent"/g),
    ).toHaveLength(2);
    expect(source).toContain('min="0"');
    expect(source).toContain('max="100"');
    expect(source).toContain("group.cache_read_reduction_percent ?? 0");
  });

  it("defaults and resets the setting to disabled", () => {
    expect(source.match(/cache_read_reduction_percent: 0/g)).toHaveLength(2);
    expect(
      source.match(/(?:create|edit)Form\.cache_read_reduction_percent = 0/g),
    ).toHaveLength(2);
  });

  it("explains that empty-cache requests are unchanged", () => {
    expect(zh.groups.form.cacheReadReductionPercentHint).toContain("无缓存时不会调整");
    expect(en.groups.form.cacheReadReductionPercentHint).toContain(
      "without cache are unchanged",
    );
  });
});
