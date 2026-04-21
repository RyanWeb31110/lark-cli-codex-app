package cmd

import (
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
	desktopCmd.AddCommand(desktopTasksCmd)
}
