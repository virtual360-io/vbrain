#!/usr/bin/env ruby
$LOAD_PATH.unshift File.expand_path("../lib", __dir__)

require "json"
require "optparse"
require "open3"
require "vbrain"

opts = { github: "none", repo_name: "vbrain" }
parser = OptionParser.new do |o|
  o.banner = "Usage: init_repo.rb [--github private|public|none] [--repo-name NAME]"
  o.on("--github V", %w[private public none]) { |v| opts[:github] = v }
  o.on("--repo-name N") { |v| opts[:repo_name] = v }
end
parser.parse!(ARGV)

data_home = VBrain::Paths.data_home
VBrain::Paths.ensure_dirs!

if VBrain::Git.repo_initialized?(data_home)
  puts JSON.generate(
    "initialized" => false, "reason" => "already a repo",
    "data_home" => data_home, "has_remote" => VBrain::Git.has_remote?(dir: data_home)
  )
  exit 0
end

if opts[:github] != "none"
  _out, _err, status = Open3.capture3("which", "gh")
  unless status.success?
    puts JSON.generate(
      "initialized" => false, "needs_gh" => true,
      "data_home" => data_home, "requested_visibility" => opts[:github]
    )
    exit 0
  end

  _, auth_err, auth_status = Open3.capture3("gh", "auth", "status")
  unless auth_status.success?
    puts JSON.generate(
      "initialized" => false, "needs_gh_auth" => true,
      "data_home" => data_home, "auth_stderr" => auth_err.to_s.strip[0, 400]
    )
    exit 0
  end
end

VBrain::Git.init!(data_home)

# Instala CLAUDE.md + skills versionadas na base e commita junto, pra base
# funcionar em qualquer ambiente que a clone (não só no ~/.claude global).
scaffold = VBrain::Scaffold.install!(data_home)
VBrain::Git.commit!("chore: assets do agente vbrain (CLAUDE.md + skills)", data_home)

remote_url = nil
if opts[:github] != "none"
  visibility_flag = "--#{opts[:github]}"
  out, err, status = Open3.capture3(
    "gh", "repo", "create", opts[:repo_name],
    visibility_flag, "--source", data_home, "--remote", "origin", "--push",
    chdir: data_home
  )
  abort("gh repo create failed: #{err.strip}\n#{out.strip}") unless status.success?

  remote_url = `git -C #{data_home.shellescape} remote get-url origin`.strip
end

puts JSON.generate(
  "initialized" => true,
  "data_home" => data_home,
  "branch" => VBrain::Git.current_branch(data_home),
  "has_remote" => !remote_url.nil?,
  "remote_url" => remote_url,
  "visibility" => opts[:github] == "none" ? nil : opts[:github],
  "claude_md" => scaffold["claude_md"],
  "skills_installed" => scaffold["skills_installed"]
)
