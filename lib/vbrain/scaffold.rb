require "fileutils"
require_relative "paths"

module VBrain
  # Instala os "assets do agente" no repo da base (~/vbrain): um CLAUDE.md que
  # instrui qualquer agente a SEMPRE usar as skills vbrain, e uma cópia
  # versionada das skills em .claude/skills/. Assim a base funciona em qualquer
  # ambiente que a clone (ex.: cloud da Anthropic), não só onde o ~/.claude
  # global já tem as skills instaladas.
  module Scaffold
    # Fonte das skills = .claude/skills/ deste repo de código (versionado).
    SKILLS_SRC = File.join(Paths::PROJECT_ROOT, ".claude", "skills").freeze

    CLAUDE_MD = <<~MD
      # CLAUDE.md — base de conhecimento vbrain

      Este repositório é a **sua base de conhecimento pessoal vbrain**: dados
      versionados (`raw/`, `wiki/`, `db/vbrain.sqlite3`, `config/`) + as skills
      do agente em `.claude/skills/`. O código Ruby vive em outro repo (o
      projeto vbrain); estas skills chamam os scripts de lá.

      ## Regra principal — SEMPRE use as skills vbrain

      Toda operação na base passa pelas skills (slash commands). **Nunca** edite
      `wiki/`, `raw/` ou `db/` na mão, nem rode SQL direto: isso quebra o índice
      e o grafo de links.

      | Quero…                                          | Use a skill                       |
      |---|---|
      | Consultar a base                                | `/vbrain-query-knowledge`         |
      | Adicionar conhecimento (arquivo/URL/nota)       | `/vbrain-add-knowledge`           |
      | Conectar fonte realtime (calendar/gmail/slack)  | `/vbrain-add-realtime-knowledge`  |
      | Criar uma rotina                                | `/vbrain-add-routine`             |
      | Rodar as rotinas (watch loop)                   | `/vbrain-routine`                 |

      As skills estão versionadas em `.claude/skills/` deste repo de propósito:
      pra funcionarem em qualquer máquina/ambiente que clone a base, não só onde
      o `~/.claude` global as tem.

      ## Pré-requisitos

      As skills são determinísticas em Ruby. Pra rodarem, o projeto de código
      vbrain precisa estar disponível, com **Ruby 3.3.6** e `bundle install`
      feito.

      ## Por quê (arquitetura)

      Wiki em markdown é a fonte da verdade; o SQLite (`db/vbrain.sqlite3`) é
      índice derivado — descartável (dá pra apagar e reconstruir com
      `reindex.rb`), mas versionado por conveniência. O LLM só entra pro que
      exige julgamento (chunkar, sintetizar páginas).
    MD

    # Escreve CLAUDE.md + copia skills. Retorna um resumo pro JSON da CLI.
    def self.install!(dir = Paths.data_home, skills_src: SKILLS_SRC)
      {
        "claude_md" => write_claude_md!(dir),
        "skills_installed" => install_skills!(dir, skills_src)
      }
    end

    # Não clobbera um CLAUDE.md já existente (o usuário pode ter customizado).
    def self.write_claude_md!(dir = Paths.data_home)
      path = File.join(dir, "CLAUDE.md")
      return false if File.exist?(path)

      File.write(path, CLAUDE_MD)
      true
    end

    # Copia cada skill (subdiretório) de skills_src para <dir>/.claude/skills/.
    # Idempotente: remove o destino de cada skill antes de copiar.
    def self.install_skills!(dir = Paths.data_home, skills_src = SKILLS_SRC)
      return 0 unless Dir.exist?(skills_src)

      dest = File.join(dir, ".claude", "skills")
      FileUtils.mkdir_p(dest)
      names = Dir.children(skills_src).select { |c| File.directory?(File.join(skills_src, c)) }
      names.each do |name|
        target = File.join(dest, name)
        FileUtils.rm_rf(target)
        FileUtils.cp_r(File.join(skills_src, name), target)
      end
      names.length
    end
  end
end
