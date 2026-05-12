import argparse
import sys
from .commands import task, project
from .core import task_manager


def main():
    prefix = task_manager.render_context_prefix()
    if prefix:
        print(prefix)
    parser = argparse.ArgumentParser(description="Quorum Agent CLI")
    subparsers = parser.add_subparsers(dest="command", help="Commands")

    # init command
    subparsers.add_parser("init", help="Initialize Quorum in the current project")

    # task subcommand
    task_parser = subparsers.add_parser("task", help="Task management")
    task_subparsers = task_parser.add_subparsers(dest="subcommand", help="Task subcommands")

    # task specify
    specify_parser = task_subparsers.add_parser("specify", help="Initialize specification session")
    specify_parser.add_argument("task_id", help="Task ID")

    # task blueprint
    blueprint_parser = task_subparsers.add_parser("blueprint", help="Generate technical blueprint")
    blueprint_parser.add_argument("task_id", help="Task ID")

    # task start
    start_parser = task_subparsers.add_parser("start", help="Start a task")
    start_parser.add_argument("task_id", help="Task ID (e.g. FEAT-001)")

    artifact_save_parser = task_subparsers.add_parser(
        "artifact-save",
        help="Persist a supported task artifact from stdin with schema validation"
    )
    artifact_save_parser.add_argument("task_id", help="Task ID")
    artifact_save_parser.add_argument("artifact_path", help="Artifact path relative to the task directory")

    # task status
    status_parser = task_subparsers.add_parser("status", help="Get task status")
    status_parser.add_argument("task_id", help="Task ID")

    # task list
    task_subparsers.add_parser("list", help="List tasks with summaries")

    # task clean
    clean_parser = task_subparsers.add_parser("clean", help="Clean up a task")
    clean_parser.add_argument("task_id", help="Task ID")
    clean_parser.add_argument(
        "--force",
        action="store_true",
        help="Discard uncommitted changes in the task worktree before removing it."
    )
    clean_parser.add_argument(
        "--save",
        action="store_true",
        help="Stash uncommitted changes (including untracked) before removing the worktree."
    )

    # task back
    back_parser = task_subparsers.add_parser(
        "back",
        help="Revert a task to its previous state (worktree, then active->inbox, or done/failed->active)"
    )
    back_parser.add_argument("task_id", help="Task ID")

    # task split
    split_parser = task_subparsers.add_parser(
        "split",
        help="Materialise child tasks from a parent's `decomposition` field (authored by /q-decompose)"
    )
    split_parser.add_argument("task_id", help="Parent task ID (e.g. FEAT-001)")

    args = parser.parse_args()

    if args.command == "init":
        project.init()
    elif args.command == "task":
        if args.subcommand == "specify":
            task.specify(args.task_id)
        elif args.subcommand == "blueprint":
            task.blueprint(args.task_id)
        elif args.subcommand == "start":
            task.start(args.task_id)
        elif args.subcommand == "artifact-save":
            task.artifact_save(args.task_id, args.artifact_path)
        elif args.subcommand == "status":
            task.status(args.task_id)
        elif args.subcommand == "list":
            task.list_all()
        elif args.subcommand == "clean":
            task.clean(args.task_id, force=args.force, save=args.save)
        elif args.subcommand == "back":
            task.back(args.task_id)
        elif args.subcommand == "split":
            task.split(args.task_id)
        else:
            task_parser.print_help()
    else:
        parser.print_help()

if __name__ == "__main__":
    main()
