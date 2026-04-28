import argparse
import sys
from .commands import task

def main():
    parser = argparse.ArgumentParser(description="Quorum Agent CLI")
    subparsers = parser.add_subparsers(dest="command", help="Commands")

    # task subcommand
    task_parser = subparsers.add_parser("task", help="Task management")
    task_subparsers = task_parser.add_subparsers(dest="subcommand", help="Task subcommands")

    # task start
    start_parser = task_subparsers.add_parser("start", help="Start a task")
    start_parser.add_argument("task_id", help="Task ID (e.g. FEAT-001)")

    # task run
    run_parser = task_subparsers.add_parser("run", help="Run a task")
    run_parser.add_argument("task_id", help="Task ID")

    # task status
    status_parser = task_subparsers.add_parser("status", help="Get task status")
    status_parser.add_argument("task_id", help="Task ID")

    # task clean
    clean_parser = task_subparsers.add_parser("clean", help="Clean up a task")
    clean_parser.add_argument("task_id", help="Task ID")

    args = parser.parse_args()

    if args.command == "task":
        if args.subcommand == "start":
            task.start(args.task_id)
        elif args.subcommand == "run":
            task.run(args.task_id)
        elif args.subcommand == "status":
            task.status(args.task_id)
        elif args.subcommand == "clean":
            task.clean(args.task_id)
        else:
            task_parser.print_help()
    else:
        parser.print_help()

if __name__ == "__main__":
    main()
