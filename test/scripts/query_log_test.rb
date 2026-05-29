require "test_helper"
require "vbrain"

class QueryLogCLITest < Minitest::Test
  PROJECT_ROOT = File.expand_path("../..", __dir__)
  QUERY     = File.join(PROJECT_ROOT, "scripts", "query.rb")
  QUERY_LOG = File.join(PROJECT_ROOT, "scripts", "query_log.rb")

  def setup
    VBrain::Paths.ensure_dirs!
    # Isola: limpa o log antes de cada teste (VBRAIN_HOME já é tmpdir).
    VBrain::DB.open { |db| db.execute("DELETE FROM query_log") }
  end

  def run_query(*args)
    Open3.capture3("bundle", "exec", "ruby", QUERY, *args, chdir: PROJECT_ROOT)
  end

  def run_log(*args)
    out, _, st = Open3.capture3("bundle", "exec", "ruby", QUERY_LOG, *args, chdir: PROJECT_ROOT)
    [JSON.parse(out), st]
  end

  def test_query_writes_a_log_row_with_count_and_source_query
    run_query("empregos", "--source-query", "quais empregos eu já tive", "--format", "json")
    dump, st = run_log("--dump")
    assert st.success?
    assert_equal 1, dump["count"]
    row = dump["entries"].first
    assert_equal "empregos", row["query"]
    assert_equal "quais empregos eu já tive", row["source_query"]
    assert_equal 0, row["results_count"], "sem páginas indexadas, 0 resultados"
  end

  def test_logs_even_when_no_valid_terms
    # Só stopwords/pontuação que normalizam pra vazio: é o sinal MAIS valioso
    # pro dream (pergunta que nem tokenizou), então tem que ser logada.
    run_query(":::", "--format", "json")
    dump, = run_log("--dump")
    assert_equal 1, dump["count"]
    assert_equal 0, dump["entries"].first["results_count"]
  end

  def test_no_log_flag_suppresses_logging
    run_query("qualquer", "--no-log", "--format", "json")
    dump, = run_log("--dump")
    assert_equal 0, dump["count"]
  end

  def test_prune_through_id_deletes_processed_and_keeps_newer
    run_query("um", "--format", "json")
    run_query("dois", "--format", "json")
    dump, = run_log("--dump")
    assert_equal 2, dump["count"]
    first_id = dump["entries"].first["id"]

    # Simula chegada de query nova DEPOIS do dump que o dream viu.
    run_query("tres", "--format", "json")

    prune, st = run_log("--prune", "--through-id", first_id.to_s)
    assert st.success?
    assert_equal 1, prune["deleted"], "só a primeira (id <= first_id)"

    after, = run_log("--dump")
    refute(after["entries"].any? { |e| e["query"] == "um" }, "processada foi apagada")
    assert(after["entries"].any? { |e| e["query"] == "tres" }, "query nova sobrevive")
  end
end
