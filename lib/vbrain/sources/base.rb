module VBrain
  module Sources
    module Base
      def detect?(_path)
        raise NotImplementedError
      end

      def kind_key
        raise NotImplementedError
      end

      def extract(_path, _out_path)
        raise NotImplementedError
      end
    end
  end
end
