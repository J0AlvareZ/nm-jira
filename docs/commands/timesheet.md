# Timesheet

## Synopsis

```sh
nm-jira timesheet [today|thisweek|lastweek]
```

`today` is used when no filter is provided. The command reads `DEFAULT_USER`
from the configured `config.toml`, resolves it to a Jira account ID, and shows
that user's worklogs.

## Time ranges

All ranges use the system's local time zone:

- `today`: the current local day.
- `thisweek`: Monday through Sunday of the current local week.
- `lastweek`: the complete Monday-through-Sunday week before the current one.

## Output

When stdout is a terminal, the command opens an interactive table. Use `q` to
quit, the arrow keys or `j`/`k` to navigate, `PgUp`/`PgDn` to move by a page,
and resize the terminal normally.
The header shows the user, filter, and grand total; each row includes its date,
issue, summary, and duration.

When stdout is piped or used in CI, the same rows and daily/grand totals are
printed as tab-separated plain text with no ANSI escape sequences.

If Jira returns only part of the data, available worklogs are still shown and a
warning is written to stderr. Pass `--verbose` to include each partial error.

## Examples

```sh
nm-jira timesheet
nm-jira timesheet thisweek
nm-jira timesheet lastweek --verbose
```
