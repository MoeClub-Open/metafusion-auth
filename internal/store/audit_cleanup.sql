DO $cleanup$
BEGIN
  IF to_regprocedure('audit.cleanup_logs(text,timestamp with time zone,text)') IS NULL THEN
    CREATE FUNCTION audit.cleanup_logs(log_kind text, cutoff timestamptz, selected_service text)
    RETURNS bigint LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog
    AS $body$
    DECLARE removed bigint;
    BEGIN
      IF cutoff IS NULL OR cutoff > date_trunc('day',now() AT TIME ZONE 'UTC') AT TIME ZONE 'UTC' THEN
        RAISE EXCEPTION 'invalid cleanup cutoff';
      END IF;
      IF log_kind = 'audit' THEN
        DELETE FROM audit.audit_log WHERE occurred_at < cutoff AND (selected_service='' OR service=selected_service);
      ELSIF log_kind = 'statistics' THEN
        DELETE FROM audit.api_statistics WHERE day < (cutoff AT TIME ZONE 'UTC')::date AND (selected_service='' OR service=selected_service);
      ELSE
        RAISE EXCEPTION 'invalid log kind';
      END IF;
      GET DIAGNOSTICS removed = ROW_COUNT;
      RETURN removed;
    END
    $body$;
    REVOKE ALL ON FUNCTION audit.cleanup_logs(text,timestamptz,text) FROM PUBLIC;
  END IF;
END
$cleanup$;
