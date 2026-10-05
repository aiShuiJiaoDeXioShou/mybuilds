package scm

import (
	"context"
	"mybuilds/internal/protocol"
	"sort"
	"strings"
	"time"
)

type ChangeOptions struct {
	Source                 Options
	TargetSHA, BaselineSHA string
}
type ChangeResult struct {
	TargetSHA, BaselineSHA string
	BaselineAvailable      bool
	Paths                  []string
	Digest                 string
}

// ReadChanges只比较授权分支内已固定的两个tree，不碰来源工作区。
func ReadChanges(parent context.Context, in ChangeOptions) (out ChangeResult, err error) {
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	if !fullOID.MatchString(in.TargetSHA) || in.BaselineSHA != "" && !fullOID.MatchString(in.BaselineSHA) || in.Source.Ref != "" && !strings.EqualFold(in.Source.Ref, in.TargetSHA) {
		return out, failure("input_invalid")
	}
	in.Source.Ref = strings.ToLower(in.TargetSHA)
	opened, err := openRepository(ctx, in.Source)
	if err != nil {
		return out, err
	}
	defer func() {
		if e := opened.Close(); e != nil {
			out = ChangeResult{}
			err = e
		}
	}()
	out = ChangeResult{TargetSHA: opened.sha, BaselineSHA: strings.ToLower(in.BaselineSHA), Paths: []string{}}
	out.Digest = protocol.ChangesDigest(out.Paths)
	if in.BaselineSHA == "" {
		return out, nil
	}
	runner := &opened.runner
	data, _, err := runner.runInput(ctx, 1024, []byte(out.BaselineSHA+"\n"), "cat-file", "--batch-check=%(objectname) %(objecttype)")
	if err != nil {
		return ChangeResult{}, err
	}
	line := strings.TrimSuffix(string(data), "\n")
	if line == out.BaselineSHA+" missing" {
		return out, nil
	}
	if line != out.BaselineSHA+" commit" {
		return ChangeResult{}, failure("ref_invalid")
	}
	data, _, err = runner.run(ctx, 8<<20, "diff", "--name-status", "-z", "--find-renames=50%", "-l1000", out.BaselineSHA, out.TargetSHA, "--")
	if err != nil {
		return ChangeResult{}, err
	}
	fields := strings.Split(string(data), "\x00")
	if fields[len(fields)-1] != "" {
		return ChangeResult{}, failure("git_failed")
	}
	fields = fields[:len(fields)-1]
	paths := map[string]bool{}
	for len(fields) > 0 {
		status := fields[0]
		fields = fields[1:]
		count := 1
		if status == "" {
			return ChangeResult{}, failure("git_failed")
		}
		switch status[0] {
		case 'R', 'C':
			count = 2
			if len(status) < 2 {
				return ChangeResult{}, failure("git_failed")
			}
			for _, r := range status[1:] {
				if r < '0' || r > '9' {
					return ChangeResult{}, failure("git_failed")
				}
			}
		case 'A', 'D', 'M', 'T', 'U', 'X', 'B':
			if len(status) != 1 {
				return ChangeResult{}, failure("git_failed")
			}
		default:
			return ChangeResult{}, failure("git_failed")
		}
		if len(fields) < count {
			return ChangeResult{}, failure("git_failed")
		}
		for _, p := range fields[:count] {
			if !protocol.ValidChangePath(p) {
				return ChangeResult{}, failure("input_invalid")
			}
			paths[p] = true
		}
		fields = fields[count:]
		if len(paths) > 100000 {
			return ChangeResult{}, failure("output_limit")
		}
		if ctx.Err() != nil {
			return ChangeResult{}, contextFailure(ctx)
		}
	}
	out.Paths = make([]string, 0, len(paths))
	for p := range paths {
		out.Paths = append(out.Paths, p)
	}
	sort.Strings(out.Paths)
	out.BaselineAvailable = true
	out.Digest = protocol.ChangesDigest(out.Paths)
	return out, nil
}
