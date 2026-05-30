require "open3"
require "fileutils"
require_relative "paths"

module VBrain
  module Git
    # O SQLite (db/vbrain.sqlite3) é um índice descartável — pode ser apagado e
    # reconstruído com scripts/reindex.rb — mas é versionado por conveniência
    # (clone/pull já trazem o índice pronto, sem precisar reindexar). Por isso
    # /db/ NÃO entra no .gitignore. Só staging volátil e lixo de SO ficam fora.
    GITIGNORE = <<~IGN.freeze
      /raw/.tmp/
      .DS_Store
    IGN

    class Error < StandardError; end

    def self.repo_initialized?(dir = Paths.data_home)
      File.directory?(File.join(dir, ".git"))
    end

    def self.init!(dir = Paths.data_home)
      FileUtils.mkdir_p(dir)
      raise Error, "repo already initialized at #{dir}" if repo_initialized?(dir)

      run!(dir, "git", "init", "-b", "main")
      write_gitignore!(dir)
      run!(dir, "git", "add", ".gitignore")
      run!(dir, "git", "commit", "-m", "chore: initialize vbrain")
      dir
    end

    def self.write_gitignore!(dir = Paths.data_home)
      path = File.join(dir, ".gitignore")
      existing = File.exist?(path) ? File.read(path) : ""
      return path if existing.include?("/raw/.tmp/")

      File.write(path, GITIGNORE)
      path
    end

    def self.add_remote!(url, dir = Paths.data_home, name: "origin")
      run!(dir, "git", "remote", "add", name, url)
    end

    def self.has_remote?(name: "origin", dir: Paths.data_home)
      out, _err, status = Open3.capture3("git", "remote", "get-url", name, chdir: dir)
      status.success? && !out.strip.empty?
    end

    def self.current_branch(dir = Paths.data_home)
      out, _err, status = Open3.capture3("git", "rev-parse", "--abbrev-ref", "HEAD", chdir: dir)
      status.success? ? out.strip : nil
    end

    def self.changes?(dir = Paths.data_home)
      out, _err, status = Open3.capture3("git", "status", "--porcelain", chdir: dir)
      status.success? && !out.strip.empty?
    end

    def self.commit!(message, dir = Paths.data_home)
      run!(dir, "git", "add", "-A")
      return { "committed" => false, "reason" => "no changes" } unless changes_staged?(dir)

      run!(dir, "git", "commit", "-m", message)
      sha = `git -C #{dir.shellescape} rev-parse HEAD`.strip
      { "committed" => true, "sha" => sha, "message" => message }
    end

    def self.changes_staged?(dir = Paths.data_home)
      _out, _err, status = Open3.capture3("git", "diff", "--cached", "--quiet", chdir: dir)
      !status.success?
    end

    def self.push!(dir = Paths.data_home, name: "origin", branch: nil)
      return { "pushed" => false, "reason" => "no remote" } unless has_remote?(name: name, dir: dir)

      branch ||= current_branch(dir)
      run!(dir, "git", "push", "-u", name, branch)
      { "pushed" => true, "remote" => name, "branch" => branch }
    end

    def self.run!(dir, *cmd)
      out, err, status = Open3.capture3(*cmd, chdir: dir)
      raise Error, "#{cmd.join(' ')} failed: #{err.strip}\n#{out.strip}" unless status.success?

      out
    end
  end
end

require "shellwords"
