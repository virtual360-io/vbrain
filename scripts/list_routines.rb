#!/usr/bin/env ruby
$LOAD_PATH.unshift File.expand_path("../lib", __dir__)

require "json"
require "optparse"
require "vbrain"

opts = { slug: nil, enabled_only: false }
parser = OptionParser.new do |o|
  o.banner = "Usage: list_routines.rb [--slug SLUG] [--enabled-only]"
  o.on("--slug SLUG")     { |v| opts[:slug] = v }
  o.on("--enabled-only")  { opts[:enabled_only] = true }
end
parser.parse!(ARGV)

routines = if opts[:slug]
             entry = VBrain::Routines.find(opts[:slug])
             entry ? [entry] : []
           elsif opts[:enabled_only]
             VBrain::Routines.enabled
           else
             VBrain::Routines.load_all
           end

puts JSON.generate(
  "config_path" => VBrain::Routines.config_path,
  "count"       => routines.size,
  "routines"    => routines
)
