#!/usr/bin/env ruby
$LOAD_PATH.unshift File.expand_path("../lib", __dir__)

require "json"
require "optparse"
require "vbrain"

opts = { slug: nil, description: "", schedule: nil, prompt: nil, prompt_file: nil, enabled: true, replace: false }
parser = OptionParser.new do |o|
  o.banner = "Usage: add_routine.rb --slug X [--description Y] [--schedule 'CRON'] (--prompt 'text' | --prompt-file PATH) [--disabled] [--replace]"
  o.on("--slug SLUG")           { |v| opts[:slug] = v }
  o.on("--description DESC")    { |v| opts[:description] = v }
  o.on("--schedule CRON")       { |v| opts[:schedule] = v }
  o.on("--prompt TEXT")         { |v| opts[:prompt] = v }
  o.on("--prompt-file PATH")    { |v| opts[:prompt_file] = v }
  o.on("--disabled")            { opts[:enabled] = false }
  o.on("--replace")             { opts[:replace] = true }
end
parser.parse!(ARGV)

abort(parser.help) if opts[:slug].nil? || opts[:slug].strip.empty?

prompt = opts[:prompt] || (opts[:prompt_file] ? File.read(opts[:prompt_file]) : nil)
abort("--prompt or --prompt-file required") if prompt.nil? || prompt.strip.empty?

begin
  entry = VBrain::Routines.add!(
    slug:        opts[:slug],
    description: opts[:description],
    schedule:    opts[:schedule],
    prompt:      prompt,
    enabled:     opts[:enabled],
    replace:     opts[:replace]
  )
rescue VBrain::Routines::Error => e
  abort(e.message)
end

puts JSON.generate(
  "config_path" => VBrain::Routines.config_path,
  "routine"     => entry,
  "total"       => VBrain::Routines.load_all.size
)
