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

- `master` is the source of truth for code and knowledge. The user writes all code on `master` and on its feature branches.
  Claude reviews code there and changes code or tests only with an explicit grant for each set of changes.
  Claude writes `knowledge/` files there as part of the agreed work. The user reviews and commits them.
- `claude-unstable` is the lab branch of Claude. Claude writes, commits and pushes freely there.
- The code of `claude-unstable` never merges into `master`. Ideas go to `master`, not code.
  The user writes an accepted idea again on `master`, or takes one named commit with an explicit `cherry-pick`.

### Cycle of `claude-unstable`

1. **Fork:** the user makes `claude-unstable` from `master` and tags the fork point `unstable/N/base`.
   Claude runs one eval on the base after a full re-import. This run is the frozen reference of the cycle.
2. **Experiment:** one hypothesis, one change, one eval run, one file in `knowledge/experiments/`.
3. **No merge of `master`** into `claude-unstable` during a cycle.
4. **Compact:** the user and Claude review the experiment files. Accepted results go to `master` as decisions, designs or backlog rows.
5. **Archive and reset:** tag `unstable/N/end`, then reset `claude-unstable` to the new `master`.
- Compact after about 3 experiments, or at once after a schema change.

### Knowledge on `claude-unstable`

- Write only new files: `knowledge/experiments/` and draft designs.
- Do not edit `decisions/`, `requirements/`, accepted designs or `index.md` on `claude-unstable`. Thus the compaction has no conflicts.

### Frozen eval on `claude-unstable`

- The eval is the yardstick. It stays as it is at `unstable/N/base` for the full cycle.
- Do not change the eval core: `internal/eval/step`, `internal/platform/evalstore`, the `eval` command, k, the metrics.
- Do not change the golden set. Each run must have the same `golden_sha256` as the base run.
- Do not change the corpus. Do a full re-import before each eval run. Check `chunks_built` in the run header.
- An eval adapter (`internal/platform/sqlite/eval`, the searcher in `cmd/cli/cmd/eval.go`) can change only when an
  interface change of the experiment forces it. Make it a separate commit `eval-adapter: <reason>`, and name it in the experiment file.
- An improvement of the eval goes to the `master` backlog. It comes into the next cycle.
