package runner

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/opengfx1030/rdnactl/internal/config"
	"github.com/opengfx1030/rdnactl/internal/recipe"
)

func startContainer(ctx context.Context, p *recipe.Plan, o StartOptions, rec *Record) (*Record, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	eng, err := resolveEngine(ctx, o.Engine, cfg)
	if err != nil {
		return nil, err
	}
	progress := o.Progress
	if progress == nil {
		progress = io.Discard
	}
	if p.Pull && !o.NoPull {
		fmt.Fprintf(progress, "%s pull %s\n", eng.Name, p.Image)
		pull := exec.CommandContext(ctx, eng.Name, "pull", p.Image)
		pull.Stdout, pull.Stderr = progress, progress
		if err := pull.Run(); err != nil {
			return nil, fmt.Errorf("%s pull %s: %w (use --no-pull to start the local image)", eng.Name, p.Image, err)
		}
	}
	digest, err := engineOut(ctx, eng.Name, "image", "inspect", "--format", "{{.Id}}", p.Image)
	if err != nil {
		return nil, fmt.Errorf("image %s is not available locally: %w", p.Image, err)
	}
	name := ContainerName(p.Name)
	// A stopped container of an earlier run keeps the name: remove it.
	_, _ = engineOut(ctx, eng.Name, "rm", "-f", name)

	argv := ContainerCommand(p, eng, rowsMount(p), cfg.ResolveHFHome())
	rec.Container, rec.Engine, rec.Image, rec.ImageDigest = name, eng.Name, p.Image, digest
	rec.Log = eng.Name + " logs " + name
	if o.Foreground {
		argv = append([]string{argv[0], "run", "--rm"}, argv[3:]...) // drop -d
		if err := writeRecord(rec); err != nil {
			return nil, err
		}
		cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
		cmd.Cancel = func() error {
			_, err := engineOut(context.Background(), eng.Name, "stop", "-t", "20", name)
			return err
		}
		cmd.Stdout, cmd.Stderr = progress, progress
		return rec, exitErr(cmd.Run())
	}
	if _, err := engineOut(ctx, argv[0], argv[1:]...); err != nil {
		return nil, err
	}
	return rec, writeRecord(rec)
}

func engineOut(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errb.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("%s %s: %s", name, args[0], msg)
	}
	return strings.TrimSpace(out.String()), nil
}
