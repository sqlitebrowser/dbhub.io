BEGIN;
CREATE INDEX IF NOT EXISTS analysis_space_used_analysis_date_index ON public.analysis_space_used (analysis_date);
CREATE INDEX IF NOT EXISTS  analysis_space_used_user_id_index ON public.analysis_space_used (user_id);
COMMIT;