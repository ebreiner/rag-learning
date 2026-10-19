package cmd

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"rag/internal/platform/evalstore"
	sqliteeval "rag/internal/platform/sqlite/eval"
	"rag/internal/platform/sqlite/querries"
	"rag/internal/platform/telemetry/logging"
	"strings"
	"text/tabwriter"
	"time"

	evalstep "rag/internal/eval/step"
	retrievalstep "rag/internal/retrieval/step"

	"github.com/spf13/cobra"
)

func NewEvalCmd() *cobra.Command {
	evalCmd := &cobra.Command{
		Use:   "eval",
		Short: "Run the golden set against the DB and compare with the previous run",
		Long: `Run each golden query for each retrieval strategy, give recall, MRR and a status
for each query (hit, ranked_out, content_lost, doc_missing), write a run file and
show the difference to the previous run.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			logger, cnf, otelShutdownFunc, err := Setup(ctx, cmd)
			if err != nil {
				return fmt.Errorf("error setting up otel and logger: %w", err)
			}
			logger = logger.With(logging.KeyStep, "eval")
			defer func() {
				if err := otelShutdownFunc(ctx); err != nil {
					logger.ErrorContext(ctx, "shutdown-err", "err", fmt.Errorf("error flushing signals and shuting down otel: %w", err))
				}
			}()

			golden, goldenSHA, err := evalstore.LoadGolden(cnf.Eval.Golden)
			if err != nil {
				return err
			}

			// the baseline is a choice for each run, so it stays a flag and is not in the config
			baselinePath := cmd.Flag("baseline").Value.String()
			runsDir := cnf.Eval.RunsDir
			k := cnf.Eval.K

			// load the baseline before Save else Latest gives the new run
			var baseline evalstep.Run
			var baselineFound bool
			if baselinePath != "" {
				baseline, err = evalstore.Load(baselinePath)
				if err != nil {
					return err
				}
				baselineFound = true
			} else {
				baseline, baselinePath, baselineFound, err = evalstore.LoadLatest(runsDir)
				if err != nil {
					return err
				}
			}

			deps, db, closeDB, err := WireUpRetrieval(ctx, cnf, logger)
			if closeDB != nil {
				defer func() {
					if err := closeDB(ctx); err != nil {
						logger.ErrorContext(ctx, "shutdown-err", "err", fmt.Errorf("error closing db: %w", err))
					}
				}()
			}
			if err != nil {
				return err
			}

			docs := sqliteeval.NewDocChunksProvider(db, logger)
			results, err := evalstep.RunEval(ctx, golden, cnf.Eval.Strategies, k, cnf.Retrieval.PoolSize, retrievalSearcher{deps: deps}, &docs)
			if err != nil {
				return fmt.Errorf("error running eval: %w", err)
			}

			chunkStats, err := querries.New(db).EvalChunkStats(ctx)
			if err != nil {
				return fmt.Errorf("error reading chunk stats: %w", err)
			}

			commit, dirty := gitState()
			run := evalstep.Run{
				Header: evalstep.RunHeader{
					Timestamp:    time.Now().UTC(),
					GitCommit:    commit,
					GitDirty:     dirty,
					GoldenPath:   cnf.Eval.Golden,
					GoldenSHA256: goldenSHA,
					DBPath:       cnf.Storage.DBPath,
					ChunkCount:   chunkStats.ChunkCount,
					ChunksBuilt:  chunkStats.LatestCreatedAt,
					EmbedModel:   cnf.Embedding.Model,
					EmbedDim:     cnf.Embedding.Dim,
					K:            k,
					PoolSize:     max(k, cnf.Retrieval.PoolSize),
				},
				Strategies: results,
			}

			runPath, err := evalstore.Save(runsDir, run)
			if err != nil {
				return err
			}

			r := newReport(cmd.OutOrStdout())
			printReport(r, run, golden)
			if baselineFound {
				printDiff(r, baseline, baselinePath, run)
			} else {
				r.line("\nno previous run, this run is the baseline\n")
			}
			r.line("\nrun file: %s\n", runPath)
			if err := r.flush(); err != nil {
				return fmt.Errorf("error writing report: %w", err)
			}

			return nil
		},
	}

	evalCmd.Flags().String("baseline", "", "--baseline eval/runs/<file>.json (default: newest file in eval.runs_dir)")

	return evalCmd
}

func gitState() (commit string, dirty bool) {
	out, err := exec.Command("git", "rev-parse", "HEAD").Output()
	if err != nil {
		return "", false
	}
	status, err := exec.Command("git", "status", "--porcelain").Output()
	if err != nil {
		return strings.TrimSpace(string(out)), false
	}
	return strings.TrimSpace(string(out)), len(strings.TrimSpace(string(status))) > 0
}

type report struct {
	tw  *tabwriter.Writer
	err error
}

func newReport(out io.Writer) *report {
	return &report{tw: tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)}
}

func (r *report) line(format string, a ...any) {
	if r.err != nil {
		return
	}
	_, r.err = fmt.Fprintf(r.tw, format, a...)
}

func (r *report) flush() error {
	if r.err != nil {
		return r.err
	}
	return r.tw.Flush()
}

func printReport(r *report, run evalstep.Run, golden evalstep.GoldenSet) {
	h := run.Header
	r.line("golden: %s (%d queries)  db: %s (%d chunks, built %s)  model: %s/%d  k: %d  pool: %d\n\n",
		h.GoldenPath, len(golden.Queries), h.DBPath, h.ChunkCount, h.ChunksBuilt, h.EmbedModel, h.EmbedDim, h.K, h.PoolSize)

	r.line("strategy\tr@1\tr@3\tr@5\tr@10\tmrr\tsect@10\tin_pool\tranked_out\tlost\tmissing\n")
	for _, s := range run.Strategies {
		m := s.Metrics
		r.line("%s\t%s\t%s\t%s\t%s\t%.3f\t%.1f\t%d\t%d\t%d\t%d\n",
			s.Strategy, recall(m.RecallAt, 1), recall(m.RecallAt, 3), recall(m.RecallAt, 5), recall(m.RecallAt, 10),
			m.MRR, m.DistinctSect, m.InPool, m.RankedOut, m.ContentLost, m.DocMissing)
	}

	r.line("\nstrategy\tdoc@1\tdoc@3\tdoc@5\tdoc@10\tdoc_mrr\n")
	for _, s := range run.Strategies {
		m := s.Metrics
		r.line("%s\t%s\t%s\t%s\t%s\t%.3f\n",
			s.Strategy, recall(m.DocRecallAt, 1), recall(m.DocRecallAt, 3), recall(m.DocRecallAt, 5), recall(m.DocRecallAt, 10), m.DocMRR)
	}

	noAnswer := map[string]bool{}
	for _, q := range golden.Queries {
		noAnswer[q.ID] = q.NoAnswer
	}

	r.line("\nmisses:\n")
	misses := 0
	for _, s := range run.Strategies {
		for _, q := range s.Queries {
			if q.Status == "" || q.Status == evalstep.StatusHit {
				continue
			}
			misses++
			if q.Status == evalstep.StatusInPool {
				r.line("  %s\t%s\t%s@%d\n", q.QueryID, s.Strategy, q.Status, q.Rank)
				continue
			}
			r.line("  %s\t%s\t%s\n", q.QueryID, s.Strategy, q.Status)
		}
	}
	if misses == 0 {
		r.line("  none\n")
	}

	r.line("\nno_answer top 1:\n")
	for _, s := range run.Strategies {
		for _, q := range s.Queries {
			if !noAnswer[q.QueryID] {
				continue
			}
			if len(q.TopK) == 0 {
				r.line("  %s\t%s\t(no result)\n", q.QueryID, s.Strategy)
				continue
			}
			top := q.TopK[0]
			r.line("  %s\t%s\t%.4f\t%s > %s\n", q.QueryID, s.Strategy, top.Score, top.DocName, top.Breadcrumb)
		}
	}
}

func printDiff(r *report, prev evalstep.Run, prevPath string, cur evalstep.Run) {
	deltas, changes, warnings := evalstep.Diff(prev, cur)

	r.line("\ncompared with %s:\n", prevPath)
	for _, w := range warnings {
		r.line("  WARNING: %s\n", w)
	}

	r.line("strategy\tΔr@1\tΔr@3\tΔr@5\tΔr@10\tΔmrr\tΔin_pool\tΔranked_out\tΔlost\tΔmissing\n")
	for _, s := range cur.Strategies {
		d, ok := deltas[s.Strategy]
		if !ok {
			continue
		}
		r.line("%s\t%s\t%s\t%s\t%s\t%+.3f\t%+d\t%+d\t%+d\t%+d\n",
			s.Strategy, recallDelta(d.RecallAt, 1), recallDelta(d.RecallAt, 3), recallDelta(d.RecallAt, 5), recallDelta(d.RecallAt, 10),
			d.MRR, d.InPool, d.RankedOut, d.ContentLost, d.DocMissing)
	}

	r.line("\nstrategy\tΔdoc@1\tΔdoc@3\tΔdoc@5\tΔdoc@10\tΔdoc_mrr\n")
	for _, s := range cur.Strategies {
		d, ok := deltas[s.Strategy]
		if !ok {
			continue
		}
		r.line("%s\t%s\t%s\t%s\t%s\t%+.3f\n",
			s.Strategy, recallDelta(d.DocRecallAt, 1), recallDelta(d.DocRecallAt, 3), recallDelta(d.DocRecallAt, 5), recallDelta(d.DocRecallAt, 10), d.DocMRR)
	}

	r.line("\nstatus changes:\n")
	if len(changes) == 0 {
		r.line("  none\n")
		return
	}
	for _, c := range changes {
		r.line("  %s\t%s\t%s → %s\n", c.QueryID, c.Strategy, c.From, c.To)
	}
}

func recall(values map[int]float64, at int) string {
	v, ok := values[at]
	if !ok {
		return "-"
	}
	return fmt.Sprintf("%.2f", v)
}

func recallDelta(values map[int]float64, at int) string {
	v, ok := values[at]
	if !ok {
		return "-"
	}
	return fmt.Sprintf("%+.2f", v)
}

type retrievalSearcher struct {
	deps retrievalstep.RetrievalDeps
}

func (s retrievalSearcher) Search(ctx context.Context, q string, k int64, strategy string) ([]evalstep.SearchHit, error) {
	got, err := retrievalstep.RunRetrieval(ctx, q, k, retrievalstep.RetrievalStrategy(strategy), retrievalstep.Filter{}, s.deps)
	if err != nil {
		return nil, err
	}

	// TODO: Text is the index text only while the renderer is a no-op
	hits := make([]evalstep.SearchHit, 0, len(got))
	for _, c := range got {
		hits = append(hits, evalstep.SearchHit{
			Chunk: evalstep.Chunk{
				ID:         c.ID,
				DocName:    c.DocTitle,
				Breadcrumb: c.Breadcrumb,
				Text:       c.Text,
			},
			Score: c.Score,
		})
	}
	return hits, nil
}
