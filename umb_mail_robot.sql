drop SEQUENCE s_mail;
drop TABLE mail;
CREATE SEQUENCE s_mail;
CREATE TABLE mail (
    id integer DEFAULT nextval('s_mail'::regclass) NOT NULL,
    sender text,
    reciever text NOT NULL,
    subject text,
    email_text text,
    store_sent int DEFAULT 0,
    content_type text default 'text/html'::text,
    sent bool default 'f',
    in_progress integer default 0,
    del integer DEFAULT 0,
    change integer,
    dt timestamp without time zone  default now()
);

insert into mail (sender,reciever,subject,email_text,content_type) values ('noreply@domain.ru','nikonor@nikonor.ru','Проверка umb_mail_robot','Привет, получатель','text/plain');

drop SEQUENCE s_mail_att;
drop TABLE mail_att;
CREATE SEQUENCE s_mail_att;
CREATE TABLE mail_att (
    id integer DEFAULT nextval('s_mail_att'::regclass) NOT NULL,
    mail_id    integer,
    doc_id     integer,
    doc_type   integer,
    name       text   ,
    attachment bytea  ,
    del        integer DEFAULT 0    
);
