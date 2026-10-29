# latex-bot

Slack bot that replies with a rendered image whenever a message contains math:

- `$...$` / `$$...$$`: LaTeX or Typst, guessed from the content. Backslash commands (`\frac`, `\alpha`) mean LaTeX; Typst-only syntax (`frac(a, b)`, `alpha`, `"text"`) means Typst. Ambiguous formulas (`x^2 + y`) are tried as LaTeX first, then Typst.
- `t$...$` / `t$$...$$`: always Typst.
- a `$` alone at the end of a line opens a display block that ends at a `$` alone on its own line. Each line becomes a row, and `&` aligns them:

      $
      x &= y^2 + z^3 \\
      x_2 &= y^3 + b
      $

When a message has more than one formula, the whole message is rendered with its text around the math (as LaTeX text or Typst strings). If that fails, for example because a message mixes LaTeX and Typst, the formulas are rendered on their own and stacked.

The image is posted to the channel by default, or as a thread reply when `LATEX_BOT_REPLY_MODE=thread`. Messages that are already inside a thread always get their reply in that thread.

Formulas that fail to compile are skipped silently, so dollar amounts and typos don't produce noise.

## sandboxing

Both `pdflatex` + `pdftoppm` and `typst` run inside `bwrap` with no network and no host files except `/usr`, `/etc/texlive`, `/var/lib/texmf` and the read-only Typst package cache, under `prlimit` (1 GB address space, 10 s CPU, 32 MB files) and a 10 second timeout. LaTeX input is also checked against a blocklist of I/O, definition and catcode primitives, because `openin_any` no longer restricts reads in TeX Live 2026.

## requirements

- Go 1.24+
- `texlive-latex texlive-amsmath texlive-amsfonts texlive-amscls texlive-preview texlive-mathtools texlive-varwidth texlive-pgf texlive-siunitx texlive-chemfig texlive-chemgreek texlive-mhchem` and `poppler-utils` (LaTeX is compiled with `pdflatex` and rasterised with `pdftoppm`)
- `typst` on `/usr/local/bin` or `/usr/bin`
- the Typst packages in `~/.local/share/latex-bot/typst-packages` (the sandbox has no network, so they are downloaded ahead of time):

      echo '#import "@preview/typsium:0.3.2": *' > /tmp/fetch.typ
      typst compile --package-cache-path ~/.local/share/latex-bot/typst-packages /tmp/fetch.typ /tmp/fetch.pdf

LaTeX formulas can use `amsmath`, `amssymb`, `amsthm`, `mathtools`, `tikz`, `siunitx`, `chemfig` and `mhchem` (`\ce{...}`). Typst formulas can use `typsium` (`ce("...")`).
- `bubblewrap`, `util-linux` (for `prlimit`)

## build and test

    go test ./...
    go build -trimpath -ldflags='-s -w' -o latex-bot .

## run

With the Slack CLI (tokens are injected automatically):

    slack run

Standalone:

    SLACK_BOT_TOKEN=xoxb-... SLACK_APP_TOKEN=xapp-... ./latex-bot

## settings

Put settings in `~/.config/latex-bot/settings` (read by the systemd unit), then `systemctl --user restart latex-bot`:

    LATEX_BOT_REPLY_MODE=channel   # or thread
    LATEX_BOT_COMMAND_UPLOAD_CHANNEL=C0123456789

## /math

`/math <formula or message>` renders math anywhere, including DMs the bot isn't in. Text without `$` is rendered as one display formula. A bot can't share files into conversations it isn't part of, so the image is uploaded to `LATEX_BOT_COMMAND_UPLOAD_CHANNEL` (a public channel the bot is in) and the reply points at that file. Everything rendered with `/math` is therefore publicly accessible, and the reply says so. Without that setting the command is disabled.
