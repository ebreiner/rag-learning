## Language

Write all LLM output in ASD-STE100 Simplified Technical English: chat replies, reports, notes,
documents, code comments and commit messages. This is permanent, not a toggle.

- Use approved STE words with their approved meaning. Technical names are permitted.
- One topic per sentence. Procedural sentences: max 20 words. Descriptive sentences: max 25 words.
- Use the imperative for instructions, the active voice and the simple tenses.
- Do not use "-ing" forms as nouns or adjectives. Do not use phrasal verbs. Use articles.
- Max 6 sentences per paragraph.
- Code, commands, identifiers and file paths stay unchanged.

## Knowledge base

Keep the project knowledge in files. It helps people and Claude instances to continue work with full context.

- When the user or another person builds something and the knowledge files need an entry, Claude writes it.
  If information for a complete entry is missing (decisions, reasons, rejected options), ask that person for it. Do not guess.
- Folder `knowledge/`: `index.md`, `decisions/`, `requirements/`, `concepts/`, `designs/`, `experiments/`. One topic per file.
  Link the files to each other.
- Each file starts with front matter: `type`, `title`, `status`, `date` or `timestamp`, `tags`.
- `index.md` lists each file with a one-line summary and shows where to start.
- **Decisions:** file `NNNN-short-name.md`. Sections: Context, Options considered, Decision, Consequences.
  Status `accepted` or `superseded`. Record a decision only after the user agrees.
  Decisions are append-only: do not edit an accepted decision. Write a new decision that supersedes it.
- **Designs:** status `draft` or `locked`. Change a locked design only with a new version or a decision.
- **Requirements:** what must be true, and why. A requirement file can collect needs when they appear:
  need, reason, where found, status.
- **Input from the product owner:** put each handover document from the product owner project into
  `requirements/`, unchanged. Put your own interpretation and the open questions in a separate file that links to it.
- **Experiments:** file `NNNN-short-name.md`. Sections: Setup, Result, Findings, Recommendation. Status `open` or `done`.
  An experiment records evidence. It is not a decision. A result becomes a decision only after the user agrees.

## Branches and ownership

- `master` is the source of truth for code and knowledge. The user does the work on feature branches from `master`
  and merges them with pull requests. The user writes the code.
- Claude reviews code, explains bugs with `file:line` and a failure case, and gives sketches in the chat.
- Claude changes code or tests only with an explicit grant for a named scope, for example "grant" or "build it".
  "Agreed" or "fine" on a proposal is not a grant. A grant for a scope includes the tests that the scope needs.
- Claude reports a bug outside the granted scope and does not fix it, also when the fix is one line.
- Claude writes `knowledge/` files and `CLAUDE.md` as part of the agreed work.
- Claude does not commit, push, merge, branch, tag or stash. The user reviews and commits all changes.
- Throwaway scripts and their tools (for example a Python venv) go into a separate worktree of Claude,
  for example `../rag-wikimeta`. Never put them into the checkout of the user.
- The `claude-unstable` lab cycle is cancelled since 2026-10-19. The branch and the tags `unstable/1/*` stay as history.

## Verification

- Use the `just` recipes: `just build`, `just test`, `just lint`, `just eval`. Report a broken recipe. Do not go around it.
- A change that can change the rank needs an eval run with `--baseline <reference run>`. The experiment file or the backlog row names both run IDs.
- Change the golden set only in a separate run. A golden change gives a new `golden_sha256`.
- Test an ingest in a scratchpad DB with a temporary config copy (absolute paths). Delete the copy after use, because it contains secrets.

## Public repo

- The repo is public. Do not commit personal, family or company data nor any corpus content: domains, `.env`, `config.yaml`,
  `eval/golden*.json`, `eval/runs/`, documents, tickets or wiki content. Do not print secret values.
- Knowledge files record what was decided and why, not which person or which Claude did the work.
  Name commits, branches, run IDs and files as evidence.
