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

- Folder `knowledge/`: `index.md`, `decisions/`, `requirements/`, `concepts/`, `designs/`. One topic per file.
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
