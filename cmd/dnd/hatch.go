package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/s0912758806p/commit-dnd/content"
	"github.com/s0912758806p/commit-dnd/internal/event"
	"github.com/spf13/cobra"
)

// `dnd hatch [--species id] [--name 名字]`：沒給參數時互動詢問（stdin 不是終端機時要求用參數）。
func newHatchCmd() *cobra.Command {
	var species, name string
	cmd := &cobra.Command{
		Use:   "hatch",
		Short: "孵出你的寵物（選物種、取名）",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, pack, err := openStore()
			if err != nil {
				return err
			}
			st, err := s.Load()
			if err != nil {
				return err
			}
			if st.Pet.Hatched() {
				return fmt.Errorf("已經有 %s 了", st.Pet.Name)
			}

			in := bufio.NewReader(cmd.InOrStdin())
			if species == "" {
				if !interactive(cmd) {
					return fmt.Errorf("請用 --species 指定物種（%s）", speciesIDs(pack))
				}
				if species, err = askSpecies(cmd, in, pack); err != nil {
					return err
				}
				if !cmd.Flags().Changed("name") {
					printf(cmd, "取個名字（直接 Enter 用物種名）：")
					line, _ := in.ReadString('\n')
					name = strings.TrimSpace(line)
				}
			}
			if _, ok := pack.SpeciesByID(species); !ok {
				return fmt.Errorf("沒有物種 %q（%s）", species, speciesIDs(pack))
			}

			r, err := s.Update(event.Event{
				At: time.Now(), Source: event.SourceUser, Type: "hatch",
				Args: map[string]string{"species": species, "name": name},
			})
			if err != nil {
				return err
			}
			for _, l := range r.Logs {
				printf(cmd, "%s\n", l.Text)
			}
			printf(cmd, "接下來在 repo 裡執行 dnd install git，commit 就會讓牠成長。\n")
			return nil
		},
	}
	cmd.Flags().StringVar(&species, "species", "", "物種 id")
	cmd.Flags().StringVar(&name, "name", "", "寵物名字（預設為物種名）")
	return cmd
}

func askSpecies(cmd *cobra.Command, in *bufio.Reader, pack *content.Pack) (string, error) {
	printf(cmd, "選一顆蛋：\n")
	for i, sp := range pack.Species {
		printf(cmd, "  %d) %s（%s）\n", i+1, sp.Name, sp.ID)
	}
	printf(cmd, "輸入編號或 id：")
	line, err := in.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	choice := strings.TrimSpace(line)
	if n, err := strconv.Atoi(choice); err == nil && n >= 1 && n <= len(pack.Species) {
		return pack.Species[n-1].ID, nil
	}
	return choice, nil
}

func speciesIDs(pack *content.Pack) string {
	ids := make([]string, len(pack.Species))
	for i, sp := range pack.Species {
		ids[i] = sp.ID
	}
	return strings.Join(ids, "、")
}

// interactive 回報 stdin 是否為終端機；測試以 SetIn 注入時視為非互動。
func interactive(cmd *cobra.Command) bool {
	f, ok := cmd.InOrStdin().(*os.File)
	if !ok {
		return false
	}
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}
