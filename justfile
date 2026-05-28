default:
    @just --list

install:
    go install .

run *ARGS:
    go run . {{ARGS}}
