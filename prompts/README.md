# Prompts

Versioned coach prompts, kept as files and never as inline strings, so changes can be reviewed and measured against the eval set. To change a prompt, add a new version, such as `coach/v2.md`, update `PROMPT_VERSION` in `coach/src/rackwise_coach/coaching.py`, and compare `evals/score.py` results.
