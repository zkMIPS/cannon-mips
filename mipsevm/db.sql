DROP TABLE IF EXISTS t_traces;
CREATE TABLE t_traces
(
    f_id           bigserial PRIMARY KEY,
    f_trace        jsonb                    NOT NULL,
    f_created_at   TIMESTAMP with time zone NOT NULL DEFAULT now()
);
