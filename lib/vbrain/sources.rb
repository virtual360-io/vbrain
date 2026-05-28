require_relative "sources/base"
require_relative "sources/text"

module VBrain
  module Sources
    REGISTRY = [Text].freeze

    def self.detect(path)
      REGISTRY.each do |src|
        return src if src.detect?(path)
      end
      nil
    end

    def self.for(kind_key)
      REGISTRY.find { |s| s.kind_key == kind_key }
    end

    def self.kinds
      REGISTRY.map(&:kind_key)
    end
  end
end
