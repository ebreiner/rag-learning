/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"fmt"
	"log"

	"rag/internal/db/connection"
	"rag/internal/retrieve"

	"github.com/spf13/cobra"
)

var testCmd = &cobra.Command{
	Use:   "test",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	Run: func(cmd *cobra.Command, args []string) {
		runTest()
	},
}

func init() {
	rootCmd.AddCommand(testCmd)
}

func runTest() {
	ctx := context.Background()
	db, err := connection.NewConn()
	if err != nil {
		log.Fatalf("error opening db: %s", err.Error())
	}

	rawQuerries := []string{
		"ich benötige zugriff auf ein system, für das ich keine zugangsdaten habe. was muss ich tun?",
		//		"welche prozesse sind teil des qualitätsmanagement?",
		//		"wie werden Live-Systeme und Server überwacht",
	}
	ftsResults := make([]retrieve.QueryResult, 0)
	for _, rawQ := range rawQuerries {
		q := retrieve.NewQuery(ctx, db)
		q.K = 25
		q.Query = rawQ
		q.Strategy = retrieve.FTS
		result, err := q.Run()
		if err != nil {
			log.Fatalf("error running fts test query %s: %s", q.CutQueryString(), err.Error())
		}
		ftsResults = append(ftsResults, result)
	}
	embeddingResults := make([]retrieve.QueryResult, 0)
	for _, rawQ := range rawQuerries {
		q := retrieve.NewQuery(ctx, db)
		q.K = 25
		q.Query = rawQ
		q.Strategy = retrieve.Embedding
		result, err := q.Run()
		if err != nil {
			log.Fatalf("error running embedding test query %s: %s", q.CutQueryString(), err.Error())
		}
		embeddingResults = append(embeddingResults, result)
	}
	hybridResults := make([]retrieve.QueryResult, 0)
	for _, rawQ := range rawQuerries {
		q := retrieve.NewQuery(ctx, db)
		q.K = 25
		q.Query = rawQ
		q.Strategy = retrieve.Hybrid
		result, err := q.Run()
		if err != nil {
			log.Fatalf("error running embedding test query %s: %s", q.CutQueryString(), err.Error())
		}
		hybridResults = append(hybridResults, result)
	}

	fmt.Println("fts results: ")
	printResult(ftsResults)

	fmt.Println("embedding results: ")
	printResult(embeddingResults)

	fmt.Println("hybrid results: ")
	printResult(hybridResults)
}

func printResult(results []retrieve.QueryResult) {
	for _, q := range results {
		fmt.Printf("query: %s:\n\n", q.Query)
		for rank := 1; rank < len(q.Results)-1; rank++ {
			chunk := q.Results[uint16(rank)]
			fmt.Printf("rank: %d, id: %d, score / distance: %f, doc: %s\n", chunk.Rank, chunk.ID, chunk.Distance, chunk.DocTitle)
		}
	}
	fmt.Println("#########################################################################")
}
