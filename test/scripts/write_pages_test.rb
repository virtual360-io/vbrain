require "test_helper"
require "vbrain"

class WritePagesCLITest < Minitest::Test
  PROJECT_ROOT = File.expand_path("../..", __dir__)
  WRITE  = File.join(PROJECT_ROOT, "scripts", "write_pages.rb")
  INGEST = File.join(PROJECT_ROOT, "scripts", "ingest_raw.rb")

  def setup
    VBrain::Paths.ensure_dirs!
    @paths_to_cleanup = []
    @sha_to_cleanup = []
  end

  def teardown
    @paths_to_cleanup.each { |p| File.delete(p) if File.exist?(p) }
    @sha_to_cleanup.each do |sha|
      VBrain::DB.open do |db|
        rows = db.execute("SELECT path FROM raw_sources WHERE sha256 = ?", [sha])
        rows.each { |r| File.delete(r["path"]) if File.exist?(r["path"]) }
        db.execute("DELETE FROM raw_sources WHERE sha256 = ?", [sha])
      end
    end
  end

  def test_writes_pages_with_resolved_slug_collision
    Dir.mktmpdir do |dir|
      src = File.join(dir, "wp_#{Time.now.to_f}.md")
      content = "marker #{Time.now.to_f}"
      File.write(src, content)
      @sha_to_cleanup << Digest::SHA256.hexdigest(File.read(src))

      stdout, _, _ = Open3.capture3("bundle", "exec", "ruby", INGEST, src, chdir: PROJECT_ROOT)
      ingest = JSON.parse(stdout)
      raw_id = ingest["raw_id"]

      tag = "writepages-test-#{Time.now.to_f.to_s.tr('.', '')}"
      pages = {
        "pages" => [
          { "kind" => "note", "title" => "WP Test", "tags" => [tag],
            "body_markdown" => "## A\n\nFirst.\n", "slug_hint" => tag },
          { "kind" => "note", "title" => "WP Test 2", "tags" => [tag],
            "body_markdown" => "## B\n\nSecond.\n", "slug_hint" => tag }
        ]
      }
      json_path = File.join(dir, "pages.json")
      File.write(json_path, JSON.generate(pages))

      stdout2, stderr2, status = Open3.capture3(
        "bundle", "exec", "ruby", WRITE,
        "--raw-id", raw_id.to_s, "--pages-json", json_path,
        chdir: PROJECT_ROOT
      )
      assert status.success?, "write_pages failed: #{stderr2}"
      result = JSON.parse(stdout2)
      assert_equal 2, result["count"]
      assert_equal 2, result["written"].uniq.size, "slugs must be unique: #{result['written']}"
      result["written"].each do |rel|
        refute_includes rel, "/", "página de conhecimento mora na raiz de wiki/ (layout plano)"
        abs = File.join(VBrain::Paths.wiki_dir, rel)
        @paths_to_cleanup << abs
        assert File.exist?(abs)
        parsed = VBrain::Page.parse(abs)
        assert_equal "note", parsed.frontmatter["kind"]
        assert_includes parsed.frontmatter["tags"], tag
      end
    end
  end

  def test_kind_defaults_to_note_when_missing_or_invalid
    Dir.mktmpdir do |dir|
      src = File.join(dir, "wp_kind_#{Time.now.to_f}.md")
      File.write(src, "marker #{Time.now.to_f}")
      @sha_to_cleanup << Digest::SHA256.hexdigest(File.read(src))

      stdout, _, _ = Open3.capture3("bundle", "exec", "ruby", INGEST, src, chdir: PROJECT_ROOT)
      raw_id = JSON.parse(stdout)["raw_id"]

      tag = "wpkind-#{Time.now.to_f.to_s.tr('.', '')}"
      pages = { "pages" => [
        { "title" => "No Kind", "tags" => [tag], "body_markdown" => "x\n", "slug_hint" => "#{tag}-a" },
        { "kind" => "garbage", "title" => "Bad Kind", "tags" => [tag], "body_markdown" => "y\n", "slug_hint" => "#{tag}-b" }
      ] }
      json_path = File.join(dir, "pages.json")
      File.write(json_path, JSON.generate(pages))

      stdout2, stderr2, status = Open3.capture3(
        "bundle", "exec", "ruby", WRITE, "--raw-id", raw_id.to_s, "--pages-json", json_path,
        chdir: PROJECT_ROOT
      )
      assert status.success?, "write_pages failed: #{stderr2}"
      JSON.parse(stdout2)["written"].each do |rel|
        abs = File.join(VBrain::Paths.wiki_dir, rel)
        @paths_to_cleanup << abs
        assert_equal "note", VBrain::Page.parse(abs).frontmatter["kind"]
      end
    end
  end

  # PORQUÊ: o writer agora pode decidir que um chunk ATUALIZA uma página
  # existente em vez de criar duplicata (o caso real: formação do LinkedIn
  # mesclada na página de formação que já existia). op:update sobrescreve o
  # corpo inteiro e mescla o frontmatter — sem isso, voltamos a gerar órfãos.
  def test_update_overwrites_existing_page_and_merges_frontmatter
    Dir.mktmpdir do |dir|
      raw_id1 = ingest_marker(dir, "u1")
      raw_id2 = ingest_marker(dir, "u2")

      slug = "wpupd-#{Time.now.to_f.to_s.tr('.', '')}"
      # 1ª ingestão cria a página com tag-a.
      write_pages(dir, raw_id1, [
        { "op" => "create", "kind" => "note", "title" => "WP Upd",
          "tags" => ["tag-a"], "body_markdown" => "# WP Upd\n\nOriginal.\n",
          "slug_hint" => slug }
      ])
      abs = File.join(VBrain::Paths.wiki_dir, "#{slug}.md")
      @paths_to_cleanup << abs

      # 2ª ingestão atualiza a MESMA página: corpo novo + tag-b.
      result = write_pages(dir, raw_id2, [
        { "op" => "update", "slug" => slug, "kind" => "note", "title" => "ignorado",
          "tags" => ["tag-b"], "body_markdown" => "# WP Upd\n\nMesclado.\n" }
      ])

      assert_equal ["#{slug}.md"], result["updated"]
      assert_empty result["written"]
      assert_equal 1, result["count"]

      parsed = VBrain::Page.parse(abs)
      assert_includes parsed.body, "Mesclado.", "corpo deve ter sido sobrescrito"
      refute_includes parsed.body, "Original.", "update reescreve o corpo inteiro"
      assert_equal "WP Upd", parsed.frontmatter["title"], "preserva título da página viva"
      assert_equal %w[tag-a tag-b], parsed.frontmatter["tags"], "union das tags"
      assert_equal 2, Array(parsed.frontmatter["source_raw"]).size, "source_raw acumula os 2 raws"
    end
  end

  # PORQUÊ: defesa anti-alucinação — se o writer apontar op:update pra um slug
  # que não existe, NÃO podemos perder o conteúdo; cai pra create (Regra 12).
  def test_update_unknown_slug_falls_back_to_create
    Dir.mktmpdir do |dir|
      raw_id = ingest_marker(dir, "ufb")
      slug = "wpghost-#{Time.now.to_f.to_s.tr('.', '')}"
      result = write_pages(dir, raw_id, [
        { "op" => "update", "slug" => slug, "kind" => "note", "title" => "Ghost",
          "tags" => ["t"], "body_markdown" => "# Ghost\n\nConteúdo.\n", "slug_hint" => slug }
      ])
      assert_empty result["updated"], "slug inexistente não pode contar como update"
      assert_equal 1, result["written"].size
      abs = File.join(VBrain::Paths.wiki_dir, result["written"].first)
      @paths_to_cleanup << abs
      assert File.exist?(abs), "fallback pra create deve persistir a página"
    end
  end

  # PORQUÊ: o dream reorganiza a wiki com autonomia total (merge/delete). O
  # delete tem que ir pelo MESMO caminho determinístico (nunca rm solto) e ser
  # atômico — move pra trash via rename e descarta. Sem isso, a wiki podia
  # ficar meio-apagada num crash, ou o dream burlaria o invariante de escrita.
  def test_delete_removes_existing_page_atomically
    Dir.mktmpdir do |dir|
      raw_id = ingest_marker(dir, "del")
      slug = "wpdel-#{Time.now.to_f.to_s.tr('.', '')}"
      keep = "wpdelkeep-#{Time.now.to_f.to_s.tr('.', '')}"
      # Segundo citador do mesmo raw pra o delete não orfanizar (guardrail).
      write_pages(dir, raw_id, [
        { "op" => "create", "kind" => "note", "title" => "WP Del",
          "tags" => ["t"], "body_markdown" => "# WP Del\n\nx\n", "slug_hint" => slug },
        { "op" => "create", "kind" => "note", "title" => "WP Del Keep",
          "tags" => ["t"], "body_markdown" => "# Keep\n\ny\n", "slug_hint" => keep }
      ])
      abs = File.join(VBrain::Paths.wiki_dir, "#{slug}.md")
      @paths_to_cleanup << File.join(VBrain::Paths.wiki_dir, "#{keep}.md")
      assert File.exist?(abs)

      result = write_pages(dir, raw_id, [{ "op" => "delete", "slug" => slug }])
      assert_equal ["#{slug}.md"], result["deleted"]
      assert_equal 1, result["count"]
      refute File.exist?(abs), "página removida da wiki"
      refute Dir.exist?(File.join(VBrain::Paths.tmp_dir, "wiki-stage-#{raw_id}")),
        "temp inteira (stage + .trash) descartada no fim do commit"
    end
  end

  def test_delete_unknown_slug_is_noop
    Dir.mktmpdir do |dir|
      raw_id = ingest_marker(dir, "delnoop")
      result = write_pages(dir, raw_id, [{ "op" => "delete", "slug" => "nao-existe-#{Time.now.to_f.to_s.tr('.', '')}" }])
      assert_empty result["deleted"], "delete idempotente: slug inexistente não falha"
      assert_equal 0, result["count"]
    end
  end

  def test_delete_skipped_when_slug_also_written_this_run
    Dir.mktmpdir do |dir|
      raw_id = ingest_marker(dir, "delconf")
      slug = "wpdelconf-#{Time.now.to_f.to_s.tr('.', '')}"
      # Contradição na mesma run: cria E manda apagar o mesmo slug. Create vence;
      # o delete é ignorado (não apagamos o que acabamos de escrever).
      result = write_pages(dir, raw_id, [
        { "op" => "create", "kind" => "note", "title" => "Keep",
          "tags" => ["t"], "body_markdown" => "x\n", "slug_hint" => slug },
        { "op" => "delete", "slug" => slug }
      ])
      abs = File.join(VBrain::Paths.wiki_dir, "#{slug}.md")
      @paths_to_cleanup << abs
      assert_empty result["deleted"], "delete de slug encenado nesta run é ignorado"
      assert File.exist?(abs), "página criada sobrevive"
    end
  end

  # PORQUÊ (guardrail): a verificação é DETERMINÍSTICA (Regra 5) — antes de
  # qualquer mv, write_pages compara os raws citados (source_raw) antes vs depois.
  # Se um delete deixaria um raw órfão (nenhuma página o cita), aborta o commit:
  # a wiki fica intacta e o resultado pede needs_review. Sem isso, o dream
  # poderia apagar a única página que cita um raw e perder a proveniência.
  def test_delete_that_would_orphan_a_raw_aborts_without_touching_wiki
    Dir.mktmpdir do |dir|
      raw_id = ingest_marker(dir, "orphan")
      slug = "wporphan-#{Time.now.to_f.to_s.tr('.', '')}"
      write_pages(dir, raw_id, [
        { "op" => "create", "kind" => "note", "title" => "Only citer",
          "tags" => ["t"], "body_markdown" => "x\n", "slug_hint" => slug }
      ])
      abs = File.join(VBrain::Paths.wiki_dir, "#{slug}.md")
      @paths_to_cleanup << abs
      raw_rel = VBrain::Page.parse(abs).frontmatter["source_raw"]

      # Apagar a única página que cita esse raw → órfão → aborta.
      json_path = File.join(dir, "del.json")
      File.write(json_path, JSON.generate("pages" => [{ "op" => "delete", "slug" => slug }]))
      stdout, _, status = Open3.capture3(
        "bundle", "exec", "ruby", WRITE, "--raw-id", raw_id.to_s, "--pages-json", json_path,
        chdir: PROJECT_ROOT
      )
      refute status.success?, "deve falhar alto quando órfanaria um raw"
      result = JSON.parse(stdout)
      assert_equal false, result["committed"]
      assert_equal true, result["needs_review"]
      assert_includes result["orphaned_raws"], raw_rel
      assert File.exist?(abs), "wiki intacta: a página NÃO foi apagada"
    end
  end

  def test_delete_commits_when_raw_still_cited_by_another_page
    Dir.mktmpdir do |dir|
      raw_id = ingest_marker(dir, "shared")
      s1 = "wpshared1-#{Time.now.to_f.to_s.tr('.', '')}"
      s2 = "wpshared2-#{Time.now.to_f.to_s.tr('.', '')}"
      # Duas páginas citam o MESMO raw (mesmo raw_id nos dois creates).
      write_pages(dir, raw_id, [{ "op" => "create", "kind" => "note", "title" => "P1",
                                  "tags" => ["t"], "body_markdown" => "x\n", "slug_hint" => s1 }])
      write_pages(dir, raw_id, [{ "op" => "create", "kind" => "note", "title" => "P2",
                                  "tags" => ["t"], "body_markdown" => "y\n", "slug_hint" => s2 }])
      abs1 = File.join(VBrain::Paths.wiki_dir, "#{s1}.md")
      abs2 = File.join(VBrain::Paths.wiki_dir, "#{s2}.md")
      @paths_to_cleanup << abs2

      # Apagar P1 é seguro: P2 ainda cita o raw.
      result = write_pages(dir, raw_id, [{ "op" => "delete", "slug" => s1 }])
      assert_equal ["#{s1}.md"], result["deleted"]
      refute File.exist?(abs1), "P1 apagada"
      assert File.exist?(abs2), "P2 sobrevive citando o raw"
    end
  end

  private

  def ingest_marker(dir, label)
    src = File.join(dir, "wp_#{label}_#{Time.now.to_f}.md")
    File.write(src, "marker #{label} #{Time.now.to_f}")
    @sha_to_cleanup << Digest::SHA256.hexdigest(File.read(src))
    stdout, _, _ = Open3.capture3("bundle", "exec", "ruby", INGEST, src, chdir: PROJECT_ROOT)
    JSON.parse(stdout)["raw_id"]
  end

  def write_pages(dir, raw_id, pages)
    json_path = File.join(dir, "pages-#{raw_id}.json")
    File.write(json_path, JSON.generate("pages" => pages))
    stdout, stderr, status = Open3.capture3(
      "bundle", "exec", "ruby", WRITE,
      "--raw-id", raw_id.to_s, "--pages-json", json_path,
      chdir: PROJECT_ROOT
    )
    assert status.success?, "write_pages failed: #{stderr}"
    JSON.parse(stdout)
  end
end
