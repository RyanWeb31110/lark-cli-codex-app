package cmd

import (
	"context"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/yjwong/lark-cli/internal/desktop"
	"github.com/yjwong/lark-cli/internal/output"
)

var desktopCmd = &cobra.Command{
	Use:   "desktop",
	Short: "Desktop GUI task queue commands",
	Long:  "Manage queued desktop GUI tasks bridged from Feishu messages.",
}

var desktopTasksCmd = &cobra.Command{
	Use:   "tasks",
	Short: "Manage queued desktop tasks",
}

var desktopHelperCmd = &cobra.Command{
	Use:   "helper",
	Short: "Run the foreground desktop helper",
	Long:  "Run the foreground desktop helper that polls the local GUI task queue and executes desktop actions from an approved GUI session.",
}

var desktopHelperPollSeconds int

var desktopHelperServeCmd = &cobra.Command{
	Use:   "serve",
	Short: "Run the foreground desktop helper loop",
	Long: `Run the foreground desktop helper loop.

Use this from a GUI-approved foreground session such as Terminal or Codex Desktop
when you want queued Feishu desktop tasks to perform real click/type actions.`,
	Run: func(cmd *cobra.Command, args []string) {
		output.JSON(map[string]any{
			"ok":           true,
			"mode":         "desktop-helper",
			"queue":        "default",
			"poll_seconds": desktopHelperPollSeconds,
		})

		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()

		worker := desktop.NewWorker(desktop.DefaultQueue(), nil, desktop.WorkerConfig{
			PollInterval: time.Duration(desktopHelperPollSeconds) * time.Second,
		})
		if err := worker.Serve(ctx); err != nil {
			output.Fatal("DESKTOP_HELPER_ERROR", err)
		}
	},
}

var desktopTasksPopCmd = &cobra.Command{
	Use:   "pop",
	Short: "Claim the oldest pending desktop task",
	Run: func(cmd *cobra.Command, args []string) {
		task, err := desktop.DefaultQueue().PopPending()
		if err != nil {
			output.Fatal("DESKTOP_TASK_ERROR", err)
		}
		if task == nil {
			output.JSON(map[string]any{
				"ok":   true,
				"task": nil,
			})
			return
		}
		output.JSON(map[string]any{
			"ok":   true,
			"task": task,
		})
	},
}

var (
	desktopTaskID          string
	desktopTaskResult      string
	desktopTaskError       string
	desktopTaskReplyResult bool
)

var desktopTasksCompleteCmd = &cobra.Command{
	Use:   "complete",
	Short: "Complete a processing desktop task",
	Run: func(cmd *cobra.Command, args []string) {
		if desktopTaskID == "" {
			output.Fatalf("VALIDATION_ERROR", "--id is required")
		}
		if desktopTaskResult == "" {
			output.Fatalf("VALIDATION_ERROR", "--result is required")
		}
		task, err := desktop.DefaultQueue().Complete(desktopTaskID, desktopTaskResult, desktopTaskReplyResult)
		if err != nil {
			output.Fatal("DESKTOP_TASK_ERROR", err)
		}
		output.JSON(map[string]any{"ok": true, "task": task})
	},
}

var desktopTasksFailCmd = &cobra.Command{
	Use:   "fail",
	Short: "Fail a processing desktop task",
	Run: func(cmd *cobra.Command, args []string) {
		if desktopTaskID == "" {
			output.Fatalf("VALIDATION_ERROR", "--id is required")
		}
		if desktopTaskError == "" {
			output.Fatalf("VALIDATION_ERROR", "--error is required")
		}
		task, err := desktop.DefaultQueue().Fail(desktopTaskID, desktopTaskError, desktopTaskReplyResult)
		if err != nil {
			output.Fatal("DESKTOP_TASK_ERROR", err)
		}
		output.JSON(map[string]any{"ok": true, "task": task})
	},
}

func init() {
	desktopTasksCompleteCmd.Flags().StringVar(&desktopTaskID, "id", "", "task id")
	desktopTasksCompleteCmd.Flags().StringVar(&desktopTaskResult, "result", "", "final result text")
	desktopTasksCompleteCmd.Flags().BoolVar(&desktopTaskReplyResult, "reply", false, "reply to the originating Feishu message")

	desktopTasksFailCmd.Flags().StringVar(&desktopTaskID, "id", "", "task id")
	desktopTasksFailCmd.Flags().StringVar(&desktopTaskError, "error", "", "error summary")
	desktopTasksFailCmd.Flags().BoolVar(&desktopTaskReplyResult, "reply", false, "reply to the originating Feishu message")

	desktopTasksCmd.AddCommand(desktopTasksPopCmd)
	desktopTasksCmd.AddCommand(desktopTasksCompleteCmd)
	desktopTasksCmd.AddCommand(desktopTasksFailCmd)

	desktopHelperServeCmd.Flags().IntVar(&desktopHelperPollSeconds, "poll-seconds", 2, "how often the helper checks for new queued tasks")
	desktopHelperCmd.AddCommand(desktopHelperServeCmd)

	desktopCmd.AddCommand(desktopTasksCmd)
	desktopCmd.AddCommand(desktopHelperCmd)
}
