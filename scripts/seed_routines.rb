#!/usr/bin/env ruby
$LOAD_PATH.unshift File.expand_path("../lib", __dir__)

require "json"
require "optparse"
require "vbrain"

# Semeia as rotinas-padrão do vbrain (hoje: `dream`) na base do usuário.
# Idempotente — ver VBrain::Routines.seed_defaults!. Roda no install.rb e pode
# ser chamado avulso. JSON no stdout.
opts = { dry_run: false }
OptionParser.new do |o|
  o.banner = "Usage: seed_routines.rb [--dry-run]"
  o.on("--dry-run") { opts[:dry_run] = true }
end.parse!(ARGV)

result = VBrain::Routines.seed_defaults!(dry_run: opts[:dry_run])

puts JSON.generate(
  "config_path" => VBrain::Routines.config_path,
  "seeded"      => result["seeded"],
  "skipped"     => result["skipped"],
  "dry_run"     => opts[:dry_run]
)
