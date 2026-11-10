# Coach evals

`positions.json` holds 17 cases: 15 single turns and 2 whole games with a `turns` list. Each has hand-written word lists, what a good answer must recommend and mention, what it must not mention, and a reference answer. The word lists are illustrative and weren't recorded from word.tips. Scores include the 40-point bonus for using every tile, as the real review does.

```sh
cd coach && source .venv/bin/activate
set -a; . ../.env; set +a          # load the API key without printing it
python ../evals/score.py --check   # no API calls: grades the reference answers
python ../evals/score.py           # 17 Claude calls, about $0.12
COACH_EFFORT=low python ../evals/score.py   # the same at low effort
```

Strategy notes are retrieved as in the app, from an in-memory copy of `notes/` with the same local embedding model, so no database is needed. Cases with a `retrieves` list must be sent at least one of those notes, and `--check` grades that too. Each result row records the note IDs sent.

Paid runs are saved in `results/`, named by prompt version and time, and each call's token use is printed. The grader is simple text matching, so read failures by hand before changing a prompt. The README's "Eval results" section has the scores so far.
