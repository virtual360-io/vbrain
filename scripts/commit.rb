#!/usr/bin/env ruby
$LOAD_PATH.unshift File.expand_path("../lib", __dir__)

require "json"
require "optparse"
require "vbrain"

opts = { message: nil, no_push: false }
parser = OptionParser.new do |o|
  o.banner = "Usage: commit.rb --message MSG [--no-push]"
  o.on("--message MSG") { |v| opts[:message] = v }
  o.on("--no-push")     { opts[:no_push] = true }
end
parser.parse!(ARGV)

abort(parser.help) if opts[:message].nil? || opts[:message].empty?

unless VBrain::Git.repo_initialized?
  puts JSON.generate("committed" => false, "pushed" => false, "reason" => "no git repo in #{VBrain::Paths.data_home}")
  exit 0
end

result = VBrain::Git.commit!(opts[:message])

push_result = if opts[:no_push] || !result["committed"]
                { "pushed" => false, "reason" => opts[:no_push] ? "no-push" : result["reason"] }
              else
                VBrain::Git.push!
              end

puts JSON.generate(result.merge(push_result))
