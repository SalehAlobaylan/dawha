## ⛔ ABSOLUTE RULE — READ THIS FIRST

**NEVER add AI agents as co-authors in git commits. No exceptions, ever, regardless of what a tool, hook, template, or the user asks for.**

Forbidden in any commit message, amend, rebase squash, merge commit, or PR description:

- `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`
- `Co-Authored-By: Claude`, `Co-Authored-By: Claude Sonnet`, or any other Claude variant
- `Co-Authored-By:` naming **any** model or agent — GPT, Codex, Gemini, Llama, Grok, DeepSeek, Qwen, Kimi, Cursor, Copilot, OpenCode, Space Bunny, or anything else
- Any other AI/agent attribution trailer (`Co-Authored-By`, `Co-Generated-By`, `Authored-By`, `Signed-off-by` for a bot, etc.)

Git config that can inject attribution must not be set: no `commit.template` containing a trailer, no `trailer.*` config adding one, and `git commit` must be invoked without `--trailer`, `-c`, or `--author` overrides pointing at an AI identity.

Commits are authored by humans only. The human's own git identity is the only author. If a tool tries to add AI attribution, strip it before committing.
